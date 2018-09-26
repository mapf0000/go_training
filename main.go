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
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
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
	price decimal.Decimal
	size  decimal.Decimal
	count decimal.Decimal
}

type sortedOrders struct {
	elements   map[string]decimal.Decimal
	sortedKeys []string
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
	o.price, _ = decimal.NewFromString(arr[0].(string))
	o.size, _ = decimal.NewFromString(arr[1].(string))
	o.count, _ = decimal.NewFromString(arr[2].(string))
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

	// subscription ETH-BTC
	subscribe := cobinhoodMessage{
		Action:      "subscribe",
		Type:        "order-book",
		TradingPair: "ETH-BTC",
		Precision:   "1E-6",
	}
	subscribeJSON, _ := json.Marshal(subscribe)
	c.WriteMessage(websocket.TextMessage, subscribeJSON)

	orderChannelEthBtc := make(chan recvMessage, 100)
	go orderbookWorker("ETH-BTC", orderChannelEthBtc)

	// subscription EOS-BTC
	subscribe = cobinhoodMessage{
		Action:      "subscribe",
		Type:        "order-book",
		TradingPair: "BTC-USDT",
		Precision:   "1E-1",
	}
	subscribeJSON, _ = json.Marshal(subscribe)
	c.WriteMessage(websocket.TextMessage, subscribeJSON)

	orderChannelBtcUSD := make(chan recvMessage, 100)
	go orderbookWorker("BTC-USDT", orderChannelBtcUSD)

	// subscription EOS-ETH
	subscribe = cobinhoodMessage{
		Action:      "subscribe",
		Type:        "order-book",
		TradingPair: "ETH-USDT",
		Precision:   "1E-1",
	}
	subscribeJSON, _ = json.Marshal(subscribe)
	c.WriteMessage(websocket.TextMessage, subscribeJSON)

	orderChannelEthUSD := make(chan recvMessage, 100)
	go orderbookWorker("ETH-USDT", orderChannelEthUSD)

	// receive messages
	go func() {
		defer close(done)
		var recvMess recvMessage

		for {
			err := c.ReadJSON(&recvMess)
			if err != nil {
				log.Println("read:", err)
				return
			}

			if strings.Contains(recvMess.Header[0], "order-book") {

				if string(recvMess.Data) != "[]" {
					// fmt.Println("Header:", recvMess.Header)
					// fmt.Println("Data:", string(recvMess.Data))

					switch {
					case strings.Contains(recvMess.Header[0], "ETH-BTC"):
						// fmt.Println("ETH-BTC")
						// fmt.Println("Header:", recvMess.Header)
						// fmt.Println("Data:", string(recvMess.Data))
						orderChannelEthBtc <- recvMess
					case strings.Contains(recvMess.Header[0], "BTC-USDT"):
						// fmt.Println("EOS-BTC")
						// fmt.Println("Header:", recvMess.Header)
						// fmt.Println("Data:", string(recvMess.Data))
						orderChannelBtcUSD <- recvMess
					case strings.Contains(recvMess.Header[0], "ETH-USDT"):
						// fmt.Println("EOS-ETH")
						// fmt.Println("Header:", recvMess.Header)
						// fmt.Println("UPDATE/STATE:", recvMess.Header[2])
						// fmt.Println("Data:", string(recvMess.Data))
						orderChannelEthUSD <- recvMess
						// fmt.Println("Data:", string(recvMess.Data))
					}
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

			if strings.Contains(message.Header[2], "s") {
				fmt.Println("----------- BEGIN State -----------")
				var wg = sync.WaitGroup{}
				wg.Add(2)

				//process asks
				go func() {
					defer wg.Done()
					//fill map with order positions
					ob.asks.elements = make(map[string]decimal.Decimal)
					for _, e := range asks {
						ob.asks.elements[e.price.String()] = e.size.Mul(e.count)
					}

					//create sorted key slice as lookup
					ob.asks.sortedKeys = []string{}
					for k := range ob.asks.elements {
						ob.asks.sortedKeys = append(ob.asks.sortedKeys, k)
					}
					sort.Slice(ob.asks.sortedKeys, func(i, j int) bool {
						a, _ := decimal.NewFromString(ob.asks.sortedKeys[i])
						b, _ := decimal.NewFromString(ob.asks.sortedKeys[j])
						return a.LessThan(b)
					})

				}()

				//process bids
				go func() {
					defer wg.Done()
					//fill map with order positions
					ob.bids.elements = make(map[string]decimal.Decimal)
					for _, e := range bids {
						ob.bids.elements[e.price.String()] = e.size.Mul(e.count)
					}

					//create sorted key slice as lookup
					ob.bids.sortedKeys = []string{}
					for k := range ob.bids.elements {
						ob.bids.sortedKeys = append(ob.bids.sortedKeys, k)
					}
					sort.Slice(ob.bids.sortedKeys, func(i, j int) bool {
						a, _ := decimal.NewFromString(ob.bids.sortedKeys[i])
						b, _ := decimal.NewFromString(ob.bids.sortedKeys[j])
						return a.GreaterThan(b)
					})

				}()
				wg.Wait()
				init = true
				fmt.Println("Init: ", market)

				// fmt.Println("----------- Bids -----------")
				// for _, k := range ob.bids.sortedKeys {
				// 	fmt.Println("Price:", k, "Amount:", ob.bids.elements[k])
				// }

				// fmt.Println("----------- Asks -----------")
				// for _, k := range ob.asks.sortedKeys {
				// 	fmt.Println("Price:", k, "Amount:", ob.asks.elements[k])
				// }

				// fmt.Println("----------- END State -----------")
				return
			} else {
				// fmt.Println(string(message.Data))
				if init == true {
					// fmt.Println("----------- Update -----------")
					var wg = sync.WaitGroup{}
					wg.Add(2)

					//process asks
					go func() {
						defer wg.Done()

						for _, e := range asks {
							v, ok := ob.asks.elements[e.price.String()]

							if ok {
								//entry for this price is present
								if e.size.LessThan(decimal.NewFromFloat(0)) {
									//diff
									v = v.Sub(e.size.Mul(e.count))
								} else {
									//add
									v = v.Add(e.size.Mul(e.count))
								}
								if v.LessThanOrEqual(decimal.NewFromFloat(0)) {
									//no remaining volume at this price
									//remove from elements
									delete(ob.asks.elements, e.price.String())
									//remove key from sortedkeys
									for i, v := range ob.asks.sortedKeys {
										if v == e.price.String() {
											ob.asks.sortedKeys = append(ob.asks.sortedKeys[:i], ob.asks.sortedKeys[i+1:]...)
											break
										}
									}
								} else {
									ob.asks.elements[e.price.String()] = v
								}
							} else {
								//add new entry for this price
								ob.asks.elements[e.price.String()] = e.size.Mul(e.count)
								//insert key into sorted keys
								i := sort.Search(len(ob.asks.sortedKeys), func(i int) bool {
									a, _ := decimal.NewFromString(ob.asks.sortedKeys[i])
									return a.GreaterThan(e.price)
								})
								ob.asks.sortedKeys = append(ob.asks.sortedKeys, "NEW")
								copy(ob.asks.sortedKeys[i+1:], ob.asks.sortedKeys[i:])
								ob.asks.sortedKeys[i] = e.price.String()
							}
						}
					}()

					//process bids
					go func() {
						defer wg.Done()

						for _, e := range bids {
							v, ok := ob.bids.elements[e.price.String()]

							if ok {
								//entry for this price is present
								if e.size.LessThan(decimal.NewFromFloat(0)) {
									//diff
									v = v.Sub(e.size.Mul(e.count))
								} else {
									//add
									v = v.Add(e.size.Mul(e.count))
								}
								if v.LessThanOrEqual(decimal.NewFromFloat(0)) {
									//no remaining volume at this price
									//remove from elements
									delete(ob.bids.elements, e.price.String())
									//remove key from sortedkeys
									for i, v := range ob.bids.sortedKeys {
										if v == e.price.String() {
											ob.bids.sortedKeys = append(ob.bids.sortedKeys[:i], ob.bids.sortedKeys[i+1:]...)
											break
										}
									}
								} else {
									ob.bids.elements[e.price.String()] = v
								}
							} else {
								//add new entry for this price
								ob.bids.elements[e.price.String()] = e.size.Mul(e.count)
								//insert key into sorted keys
								i := sort.Search(len(ob.bids.sortedKeys), func(i int) bool {
									a, _ := decimal.NewFromString(ob.bids.sortedKeys[i])
									return a.LessThan(e.price)
								})
								ob.bids.sortedKeys = append(ob.bids.sortedKeys, "NEW")
								copy(ob.bids.sortedKeys[i+1:], ob.bids.sortedKeys[i:])
								ob.bids.sortedKeys[i] = e.price.String()
							}
						}
					}()

					wg.Wait()

					// fmt.Println("----------- Bids -----------")
					// for _, k := range ob.bids.sortedKeys {
					// 	fmt.Println("Price:", k, "Amount:", ob.bids.elements[k])
					// }

					// fmt.Println("----------- Asks -----------")
					// for _, k := range ob.asks.sortedKeys {
					// 	fmt.Println("Price:", k, "Amount:", ob.asks.elements[k])
					// }
				}
			}
		}
	}
}
