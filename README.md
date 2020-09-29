# go-websockets-multicast
golang package provide multicast sending messages to all connected clients

Usage:
```go
package main

import (
    multicast "github.com/asmyasnikov/go-websockets-multicast"
    "github.com/gorilla/websocket"
	"net/http"
    "fmt"
    "time"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize: 128,
		WriteBufferSize: 128,
	}
)

func main() {
    m := multicast.New(nil, true)
    go func() {
        for {
            m.SendAll(time.Now())
            time.Sleep(time.Second)
        }
    }()
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println(err)
			return
		}
		m.Add(conn)
    })
	if err := http.ListenAndServe(":80", nil); err != nil {
		fmt.Println(err)
	}
}
```
