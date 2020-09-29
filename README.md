# go-websockets-multicast
golang package provide multicast sending messages to all connected clients

Usage:
```go

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
    }
    http.Handle("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Caller().Err(err).Msg("Connection")
			return
		}
		m.Add(conn)
    })
	if err := http.ListenAndServe(":80", nil); err != nil {
		fmt.Println(err)
	}
}
```
