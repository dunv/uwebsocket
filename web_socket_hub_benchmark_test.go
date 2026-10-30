package uwebsocket

import (
	"context"
	"fmt"
	"testing"

	"github.com/dunv/uhttp"
	"github.com/gorilla/websocket"
)

func BenchmarkHubSend(b *testing.B) {
	for _, subscribers := range []int{0, 1, 100} {
		b.Run(fmt.Sprintf("subscribers=%d", subscribers), func(b *testing.B) {
			h := NewWebSocketHub(uhttp.NewUHTTP(), websocket.TextMessage, context.Background()).(*webSocketHub)
			clients := make([]*webSocketClient, subscribers)
			for i := range clients {
				id := fmt.Sprint(i)
				clients[i] = &webSocketClient{clientGUID: id, attributes: NewClientAttributes(), send: make(chan []byte, 1)}
				h.clients[id] = &hubClient{client: clients[i], guid: id, attrs: clients[i].attributes, send: clients[i].send, active: true}
			}
			payload := make([]byte, 1024)
			builds := 0
			opts := []SendOption{WithMessageFn(func() ([]byte, error) {
				builds++
				return payload, nil
			})}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.Send(opts...)
				for _, c := range clients {
					<-c.send
				}
			}
			b.StopTimer()
			if subscribers == 0 && builds != 0 || subscribers > 0 && builds != b.N {
				b.Fatalf("unexpected payload builds: %d", builds)
			}
		})
	}
}
