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
	price int64
	size  int64
	count int64
}

type sortedOrders struct {
	elements   map[int64]int64
	sortedKeys []int64
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
	o.price = parseIntMoney(arr[0].(string))
	o.size, _ = strconv.ParseInt(arr[1].(string), 10, 64)
	o.count = parseIntMoney(arr[2].(string))
	return nil
}

// func (o *orderbookPosition) UnmarshalJSON(bs []byte) error {
// 	arr := []interface{}{}
// 	json.Unmarshal(bs, &arr)
// 	// TODO: add error handling here.
// 	o.Price, _ = strconv.ParseFloat(arr[0].(string), 64)
// 	o.Size, _ = strconv.Atoi(arr[1].(string))
// 	o.Count, _ = strconv.ParseFloat(arr[2].(string), 64)
// 	return nil
// }

func parseIntMoney(s string) int64 {
	fmt.Println(s)
	if strings.Contains(s, ".") {
		s = strings.Replace(s, ".", "", -1)
	} else {
		s = s + "000000"
	}

	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}

func formatDecimal(amount int64, precision int) string {
	// Work with absolute amount value
	var abs int64
	if amount < 0 {
		abs = -amount
	} else {
		abs = amount
	}
	sa := strconv.FormatInt(abs, 10)

	if len(sa) <= precision {
		sa = strings.Repeat("0", precision-len(sa)+1) + sa
	}

	if precision > 0 {
		sa = sa[:len(sa)-precision] + "." + sa[len(sa)-precision:]
	}

	// Add minus sign for negative amount
	if amount < 0 {
		sa = "-" + sa
	}

	return sa
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
	precision := 6
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
					ob.asks.elements = make(map[int64]int64)
					for _, e := range asks {
						ob.asks.elements[e.price] = e.count * e.size
					}

					//create sorted key slice as lookup
					ob.asks.sortedKeys = []int64{}
					for k := range ob.asks.elements {
						ob.asks.sortedKeys = append(ob.asks.sortedKeys, k)
					}
					sort.Slice(ob.asks.sortedKeys, func(i, j int) bool {
						return ob.asks.sortedKeys[i] < ob.asks.sortedKeys[j]
					})

				}()

				//process bids
				go func() {
					defer wg.Done()
					//fill map with order positions
					ob.bids.elements = make(map[int64]int64)
					for _, e := range bids {
						ob.bids.elements[e.price] = e.size * e.count
					}

					//create sorted key slice as lookup
					ob.bids.sortedKeys = []int64{}
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
					fmt.Println("Price:", formatDecimal(k, precision), "Amount:", formatDecimal(ob.bids.elements[k], precision))
				}

				fmt.Println("----------- Asks -----------")
				for _, k := range ob.asks.sortedKeys {
					fmt.Println("Price:", formatDecimal(k, precision), "Amount:", formatDecimal(ob.asks.elements[k], precision))
				}

				return

				fmt.Println("----------- END State -----------")
			} else {
				fmt.Println(string(message.Data))
				if init == true {
					fmt.Println("----------- Update -----------")
					var wg sync.WaitGroup
					wg.Add(2)

					//process asks
					go func() {
						defer wg.Done()

						for _, e := range asks {
							v, ok := ob.asks.elements[e.price]

							if ok {
								//entry for this price is present
								if e.size < 0 {
									//diff
									v -= -1 * e.size * e.count
								} else {
									//add
									v += e.size * e.count
								}
								if v <= 0 {
									//no remaining volume at this price
									//remove from elements
									delete(ob.asks.elements, e.price)
									//remove key from sortedkeys
									for i, v := range ob.asks.sortedKeys {
										if v == e.price {
											ob.asks.sortedKeys = append(ob.asks.sortedKeys[:i], ob.asks.sortedKeys[i+1:]...)
											break
										}
									}
								} else {
									ob.asks.elements[e.price] = v
								}
							} else {
								//add new entry for this price
								ob.asks.elements[e.price] = e.size * e.count
								//insert key into sorted keys
								i := sort.Search(len(ob.asks.sortedKeys), func(i int) bool { return ob.asks.sortedKeys[i] > e.price })
								ob.asks.sortedKeys = append(ob.asks.sortedKeys, 0)
								copy(ob.asks.sortedKeys[i+1:], ob.asks.sortedKeys[i:])
								ob.asks.sortedKeys[i] = e.price
							}
						}
					}()

					//process bids
					go func() {
						defer wg.Done()

						for _, e := range bids {
							v, ok := ob.bids.elements[e.price]

							if ok {
								//entry for this price is present
								delete(ob.bids.elements, e.price)
								if e.size < 0 {
									//diff
									v -= -1 * e.size * e.count
								} else {
									//add
									v += e.size * e.count
								}
								if v <= 0 {
									//no remaining volume at this price
									//remove from elements
									delete(ob.bids.elements, e.price)
									//remove key from sortedkeys
									for i, v := range ob.bids.sortedKeys {
										if v == e.price {
											ob.bids.sortedKeys = append(ob.bids.sortedKeys[:i], ob.bids.sortedKeys[i+1:]...)
											break
										}
									}
								} else {
									ob.bids.elements[e.price] = v
								}
							} else {
								//add new entry for this price
								ob.bids.elements[e.price] = e.size * e.count
								//insert key into sorted keys
								i := sort.Search(len(ob.bids.sortedKeys), func(i int) bool { return ob.bids.sortedKeys[i] < e.price })
								ob.bids.sortedKeys = append(ob.bids.sortedKeys, 0)
								copy(ob.bids.sortedKeys[i+1:], ob.bids.sortedKeys[i:])
								ob.bids.sortedKeys[i] = e.price
							}
						}
					}()

					wg.Wait()

					// fmt.Println("----------- Bids -----------")
					// for _, k := range ob.bids.sortedKeys {
					// 	fmt.Println("Price:", formatDecimal(k, precision), "Amount:", formatDecimal(ob.bids.elements[k], precision))
					// }

					// fmt.Println("----------- Asks -----------")
					// for _, k := range ob.asks.sortedKeys {
					// 	fmt.Println("Price:", formatDecimal(k, precision), "Amount:", formatDecimal(ob.asks.elements[k], precision))
					// }
				}
			}
		}
	}
}
