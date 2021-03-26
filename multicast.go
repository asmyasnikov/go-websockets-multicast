// Package multicast provides multicast
// helpers over websocket.
package multicast

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	_SET_DELAY = "_DELAY="
	MIN_DELAY  = time.Duration(1000/24) * time.Millisecond
	MAX_DELAY  = time.Second
)

type Multicast struct {
	sync.RWMutex
	connections map[*websocket.Conn]chan []byte
	channels    map[chan []byte]chan struct{}
	snapshot    interface{}
	ignoreDelay bool
}

// Creates new Multicast with empty connections and channels
func New(snapshot interface{}, ignoreDelays ...bool) *Multicast {
	return &Multicast{
		connections: make(map[*websocket.Conn]chan []byte),
		channels:    make(map[chan []byte]chan struct{}),
		snapshot:    snapshot,
		ignoreDelay: func() bool {
			if len(ignoreDelays) > 0 {
				return ignoreDelays[0]
			}
			return false
		}(),
	}
}

// Json get diff of object if its possible otherwise marshal full object
func (m *Multicast) Json(data interface{}) ([]byte, error) {
	if result, ok := data.([]byte); ok {
		return result, nil
	}

	return json.Marshal(&data)
}

// SendAll sends diff data for all connections. If diff is empty, than data will not be sent
func (m *Multicast) SendAll(data interface{}) error {
	m.RLock()
	defer m.RUnlock()
	if len(m.connections) == 0 {
		return nil
	}

	sendData, err := m.Json(data)

	if err != nil {
		return err
	}

	if sendData == nil {
		return nil
	}

	m.updateSnapshot(data)

	for conn, send := range m.connections {
		hardWork := m.channels[send]
		select {
		case _, ok := <-hardWork:
			if !ok {
				delete(m.channels, send)
				delete(m.connections, conn)
			}
		default:
			send <- sendData
		}
	}

	return nil
}

// updateSnapshot updates snapshot in multicast
func (m *Multicast) updateSnapshot(snapshot interface{}) {
	m.snapshot = snapshot
}

func waitDone(send chan<- []byte, done <-chan struct{}) {
	for {
		_, ok := <-done
		if !ok {
			close(send)
			return
		}
	}
}

// Add creates worker, that handle websocket data sending and receiving
// Returns channels of receive data from connection
// Closing of receive channel are equal for closing connection
func (m *Multicast) Add(conn *websocket.Conn) <-chan []byte {
	send, receive, done := m.add(conn)
	go waitDone(send, done)
	return receive
}

// add creates worker, that handle websocket data sending and receiving
// Returns channels to send, receive data from connection and channel work, that explains if worker is working
func (m *Multicast) add(conn *websocket.Conn) (chan<- []byte, <-chan []byte, <-chan struct{}) {
	send := make(chan []byte, 20)
	receives := make(chan []byte, 20)
	work := make(chan struct{}, 1)
	hardWork := make(chan struct{}, 1)
	m.Lock()
	if m.snapshot != nil {
		if b, err := m.Json(m.snapshot); err != nil {
			log.Error().Caller().Err(err).Msg("")
		} else {
			send <- b
		}
	}
	m.connections[conn] = send
	m.channels[send] = hardWork
	m.Unlock()
	go worker(conn, send, receives, work, hardWork, m.ignoreDelay)
	return send, receives, work
}

func processManageFlags(conn *websocket.Conn, data []byte, delay *int64) error {
	if delay == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	if data[0] != '_' {
		return nil
	}
	if len(data) > len(_SET_DELAY) && string(data[:len(_SET_DELAY)]) == _SET_DELAY {
		v, err := strconv.ParseFloat(string(data[len(_SET_DELAY):]), 64)
		if err != nil {
			log.Error().Caller().Err(err).Msg("")
			return err
		}
		d := time.Duration(v) * time.Millisecond
		if conn != nil {
			log.Info().Caller().Dur("delay", d).Str("remote", conn.RemoteAddr().String()).Msg("client-request")
		} else {
			log.Info().Caller().Dur("delay", d).Msg("client-request")
		}
		if d > MAX_DELAY || d < MIN_DELAY {
			d = time.Duration(math.Min(float64(MAX_DELAY.Milliseconds()), math.Max(float64(MIN_DELAY.Milliseconds()), float64(d.Milliseconds())))) * time.Millisecond
			log.Warn().Caller().Dur("delay", d).Msg("fixed")
		}
		atomic.StoreInt64(delay, d.Milliseconds())
	}
	return nil
}

// worker handle receive and send data from/to conn
func worker(conn *websocket.Conn, send <-chan []byte, receives chan<- []byte, work chan<- struct{}, hardWork chan<- struct{}, ignoreDelay bool) {
	delay := func() int64 {
		if ignoreDelay {
			return time.Minute.Milliseconds()
		}
		return MIN_DELAY.Milliseconds()
	}()
	once := sync.Once{}
	closeConn := func() {
		once.Do(func() {
			work <- struct{}{}
			close(hardWork)
			close(work)
			close(receives)
			if err := conn.Close(); err != nil {
				log.Error().Caller().Err(err).Msg("")
			}
		})
	}

	go func() {
		defer closeConn()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				log.Error().Caller().Err(err).Msg("")
				return
			}
			if len(data) == 0 {
				continue
			}
			if data[0] == '_' {
				if err := processManageFlags(conn, data, func() *int64 {
					if ignoreDelay {
						return nil
					}
					return &delay
				}()); err != nil {
					log.Error().Caller().Err(err).Msg("")
				}
			}
			receives <- data
		}
	}()

	go func() {
		defer closeConn()
		next := time.Now()
		snapshot := make([]byte, 0)
		for {
			select {
			case message, ok := <-send:
				if !ok {
					return
				}
				if len(message) == 0 {
					continue
				}
				if ignoreDelay {
					if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
						log.Error().Caller().Err(err).Msg("")
						return
					}
					snapshot = snapshot[:0]
				} else {
					snapshot = append(snapshot[:0], message...)
				}
			case <-time.After(time.Until(next)):
				if len(snapshot) > 0 {
					if err := conn.WriteMessage(websocket.TextMessage, snapshot); err != nil {
						log.Error().Caller().Err(err).Msg("")
						return
					}
					snapshot = snapshot[:0]
				}
				next = time.Now().Add(time.Duration(atomic.LoadInt64(&delay)) * time.Millisecond)
			}
		}
	}()
}
