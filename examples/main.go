package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dunv/uhttp"
	"github.com/dunv/ulog"
	ws "github.com/dunv/uwebsocket"
	"github.com/google/uuid"
	"golang.org/x/net/context"
)

func main() {
	// implementation for an echo-server

	// can be tested with
	// websocat ws://localhost:8080/ws

	u := uhttp.NewUHTTP(
		uhttp.WithAddress("0.0.0.0:8080"),
	)

	inboundMessages := make(chan ws.ClientMessage)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wsHub := ws.CreateHubAndRunInBackground(u, &inboundMessages, ctx)

	go func() {
		for inboundMessage := range inboundMessages {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			ulog.LogIfError(wsHub.SendToClientSync(
				inboundMessage.ClientGUID,
				[]byte(fmt.Sprintf(`{ "msg": "response to %s" }`, inboundMessage.Message)),
				ctx,
			))
			cancel()
		}
	}()

	go func() {
		for {
			input := ""
			_, err := fmt.Scanln(&input)
			if err != nil {
				ulog.Errorf("could not scan (%s)", err)
				continue
			}

			marshaled, err := json.Marshal(map[string]string{"received": input})
			if err != nil {
				ulog.Errorf("could not marshal (%s)", err)
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			ulog.LogIfError(wsHub.SendWithFilterSync(func(clientGUID string, attrs *ws.ClientAttributes) bool {
				return true
			}, marshaled, ctx))
			cancel()
		}
	}()

	wsHub.Handle("/ws", WsHandler)
	ulog.Fatal(u.ListenAndServe()) // need to investigate, why does this not work with localhost
}

var WsHandler = &ws.Handler{
	ClientAttributes: ws.ClientAttributesFunc(func(hub *ws.WebSocketHub, r *http.Request) (*ws.ClientAttributes, error) {
		return ws.NewClientAttributes().SetString("testGuid", uuid.New().String()), nil
	}),
	WelcomeMessage: ws.WelcomeMessage(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request) ([]byte, error) {
		return []byte(fmt.Sprintf(`{"msg": "Welcome, %s"}`, clientGuid)), nil
	}),
	OnConnect: ws.OnConnect(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request) {
		ulog.Infof("Client connected %v", clientGuid)
	}),
	OnDisconnect: ws.OnDisconnect(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request, err error) {
		ulog.Infof("Client disonnected %v", clientGuid)
	}),
}
