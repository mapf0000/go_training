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

type orderbookData struct {
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

	go func() {
		defer close(done)

		var objmap map[string]*json.RawMessage
		//err := json.Unmarshal(data, &objmap)

		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			json.Unmarshal(message, &objmap)

			var header []string
			err = json.Unmarshal(*objmap["h"], &header)

			//log.Printf("recv: header: %s, data: %s", header, string(*objmap["d"]))

			if strings.Contains(header[0], "order-book") {
				log.Printf("recv: header: %s, data: %s", header, string(*objmap["d"]))
			}
		}
	}()

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
