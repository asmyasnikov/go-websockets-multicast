package multicast

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProcessManageFlags(t *testing.T) {
	delay := int64(-1)
	require.NoError(t, processManageFlags(nil, []byte("_DELAY=200"), &delay))
	require.Equal(t, int64(200), delay)
}

func TestWaitDone(t *testing.T) {
	send := make(chan []byte, 20)
	work := make(chan struct{}, 1)
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		waitDone(send, work)
	}()
	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		work <- struct{}{}
		time.Sleep(time.Millisecond * 10)
		close(work)
	}()
	wg.Wait()
}

func TestMulticast_UpdateSnapshot(t *testing.T) {
	var (
		field11 = 10
		field21 = "test"
		field12 = 15
		field22 = "test1"
	)

	snapshot1 := *NewTestData(&field11, &field21)
	snapshot2 := *NewTestData(&field12, &field22)
	m := New(snapshot1)
	require.Equal(t, snapshot1, m.snapshot)

	m.updateSnapshot(snapshot2)
	require.Equal(t, snapshot2, m.snapshot)
}

func TestMulticast_Json(t *testing.T) {
	var (
		field1 = 10
		field2 = "test"
	)

	snapshot := *NewTestData(&field1, &field2)
	m := New(snapshot)
	b, err := m.Json(snapshot)
	require.NoError(t, err)
	require.Equal(t, `{"field_1":10,"field_2":"test"}`, string(b))
}

func TestMulticast_Send(t *testing.T) {
	snapshot := *NewEmptyData()
	m := New(snapshot)
	require.Equal(t, snapshot, m.snapshot)

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{
				ReadBufferSize:  128,
				WriteBufferSize: 128,
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				require.NoError(t, err)
				return
			}
			_ = m.Add(conn)
		}),
	)
	var (
		field1 = 20
		field2 = "tset"
	)
	url := "ws" + strings.TrimPrefix(s.URL, "http")
	wg := sync.WaitGroup{}
	dialerCount := 100
	sendAllData := *NewTestData(&field1, &field2)
	expected := []TestData{snapshot, sendAllData, sendAllData}
	for i := 0; i < dialerCount; i++ {
		wg.Add(len(expected) - 2)
		go func(t *testing.T, wg *sync.WaitGroup, url string, expected []TestData) {
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			require.NoError(t, err, "Dial error for socket "+url)
			defer conn.Close()
			for i := 0; i < len(expected); i++ {
				_, p, err := conn.ReadMessage()
				require.NoError(t, err, "Read error")

				data, err := json.Marshal(expected[i])
				require.NoError(t, err)

				require.Equal(t, data, p, "Get message failed")
				wg.Done()
			}
		}(t, &wg, url, expected)
	}
	wg.Wait()
	wg.Add(dialerCount * 2)
	m.SendAll(sendAllData)
	data, err := json.Marshal(sendAllData)
	require.NoError(t, err)
	time.Sleep(MIN_DELAY)
	m.SendAll(data)
	s.Close()
	wg.Wait()
}
