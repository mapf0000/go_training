package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
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

type orderPosting struct {
	Price float32
	Size  int
	Count float32
}

type orderbookPosting struct {
	Bids []orderPosting `json:"bids"`
	Asks []orderPosting `json:"asks"`
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
	print(string(pingJSON))

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
			json.Unmarshal(message, &recvMess)

			if strings.Contains(recvMess.Header[0], "order-book") {
				orderChannel <- recvMess
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

func orderbookWorker(tradingPair string, recvChannel <-chan recvMessage) {
	var orderbookPos orderbookPosting

	for message := range recvChannel {
		json.Unmarshal(message.Data, &orderbookPos)

		for _, elem := range orderbookPos.Asks {
			print(elem.Price)
		}
	}
}
