package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", "ws.cobinhood.com", "http service address")

type cobinhoodMessage struct {
	Action      string `json:"action,omitempty"`
	Type        string `json:"type,omitempty"`
	TradingPair string `json:"trading_pair_id,omitempty"`
	Precision   string `json:"precision,omitempty"` //for orderbooks
	Timeframe   string `json:"timeframe,omitempty"` //for candles
}

type recvMessage struct {
	Header []string        `json:"h"`
	Data   json.RawMessage `json:"d"`
}

type orderbookPosition struct {
	Price float64
	Size  float64
	Count float64
}

type sortedOrders struct {
	elements   map[float64]float64
	sortedKeys []float64
}

type orderbook struct {
	market string
	bids   sortedOrders
	asks   sortedOrders
}

func (o *orderbookPosition) UnmarshalJSON(bs []byte) error {
	arr := []interface{}{}
	json.Unmarshal(bs, &arr)
	// TODO: add error handling here.
	o.Price, _ = strconv.ParseFloat(arr[0].(string), 64)
	o.Size, _ = strconv.ParseFloat(arr[1].(string), 64)
	o.Count, _ = strconv.ParseFloat(arr[2].(string), 64)
	return nil
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "wss", Host: *addr, Path: "/v2/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	ping := cobinhoodMessage{
		Action: "ping",
	}
	pingJSON, _ := json.Marshal(ping)

	subscribe := cobinhoodMessage{
		Action:      "subscribe",
		Type:        "order-book",
		TradingPair: "ETH-BTC",
		Precision:   "1E-6",
	}
	subscribeJSON, _ := json.Marshal(subscribe)
	c.WriteMessage(websocket.TextMessage, subscribeJSON)

	orderChannel := make(chan recvMessage, 100)
	go orderbookWorker("ETH-BTC", orderChannel)

	// receive messages
	go func() {
		defer close(done)
		var recvMess recvMessage

		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			//fmt.Println(string(message))
			json.Unmarshal(message, &recvMess)

			if strings.Contains(recvMess.Header[0], "order-book") {

				if string(recvMess.Data) != "[]" {
					//fmt.Println("Header:", recvMess.Header)
					//fmt.Println("Data:", string(recvMess.Data))
					orderChannel <- recvMess
				}

			}
		}
	}()

	// send messages
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			c.WriteMessage(websocket.TextMessage, pingJSON)
			if err != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

func orderbookWorker(market string, recvChannel <-chan recvMessage) {
	init := false
	ob := orderbook{}
	ob.market = market

	for message := range recvChannel {
		objmap := map[string]*json.RawMessage{}
		bids := []orderbookPosition{}
		asks := []orderbookPosition{}

		err := json.Unmarshal(message.Data, &objmap)
		if err != nil {
			fmt.Println(err)
		} else {
			json.Unmarshal(*objmap["bids"], &bids)
			json.Unmarshal(*objmap["asks"], &asks)

			if message.Header[2] == "s" {
				fmt.Println("----------- BEGIN State -----------")
				var wg sync.WaitGroup
				wg.Add(2)

				//process asks
				go func() {
					defer wg.Done()
					//fill map with order positions
					ob.asks.elements = make(map[float64]float64)
					for _, e := range asks {
						ob.asks.elements[e.Price] = e.Size * e.Count
					}

					//create sorted key slice as lookup
					ob.asks.sortedKeys = []float64{}
					for k := range ob.asks.elements {
						ob.asks.sortedKeys = append(ob.asks.sortedKeys, k)
					}
					sort.Float64s(ob.asks.sortedKeys)

				}()

				//process bids
				go func() {
					defer wg.Done()
					//fill map with order positions
					ob.bids.elements = make(map[float64]float64)
					for _, e := range bids {
						ob.bids.elements[e.Price] = e.Size * e.Count
					}

					//create sorted key slice as lookup
					ob.bids.sortedKeys = []float64{}
					for k := range ob.bids.elements {
						ob.bids.sortedKeys = append(ob.bids.sortedKeys, k)
					}
					sort.Slice(ob.bids.sortedKeys, func(i, j int) bool {
						return ob.bids.sortedKeys[i] > ob.bids.sortedKeys[j]
					})

				}()
				wg.Wait()
				init = true

				fmt.Println("----------- Bids -----------")
				for _, k := range ob.bids.sortedKeys {
					fmt.Println("Price:", k, "Amount:", ob.bids.elements[k])
				}

				fmt.Println("----------- Asks -----------")
				for _, k := range ob.asks.sortedKeys {
					fmt.Println("Price:", k, "Amount:", ob.asks.elements[k])
				}

				fmt.Println("----------- END State -----------")
			} else {
				if init == true {
					fmt.Println("----------- Update -----------")

					//process bids
					for _, e := range bids {
						v, ok := ob.bids.elements[e.Price]
						if ok {
							//entry for this price is present
							v -= e.Size * e.Count
							if v == 0 {
								delete(ob.bids.elements, e.Price)
								//TODO remove key from sortedkeys
							} else {
								ob.bids.elements[e.Price] = v
							}
						} else {
							//new entry for this price has to be added

						}

					}
				}
			}
		}
	}
}
