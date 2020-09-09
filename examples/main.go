package main

import (
	"fmt"
	"net/http"

	"github.com/dunv/uhelpers"
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
			ulog.LogIfError(wsHub.SendWithFilter(
				func(attrs ws.ClientAttributes) bool {
					return attrs["clientGuid"] == inboundMessage.Client["clientGuid"]
				},
				[]byte(fmt.Sprintf("response to %s", inboundMessage.Message)),
			))
		}
	}()

	wsHub.Handle("/ws", WsHandler)
	ulog.Fatal(u.ListenAndServe()) // need to investigate, why does this not work with localhost
}

var WsHandler = &ws.Handler{
	ClientAttributes: ws.ClientAttributesFunc(func(hub *ws.WebSocketHub, r *http.Request) (ws.ClientAttributes, error) {
		clientAttributeMap := map[string]interface{}{"clientGuid": uhelpers.PtrToString(uuid.New().String())}
		return clientAttributeMap, nil
	}),
	WelcomeMessage: ws.WelcomeMessage(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes ws.ClientAttributes, r *http.Request) ([]byte, error) {
		return []byte(fmt.Sprintf(`{"msg": "Welcome, %s"}`, clientGuid)), nil
	}),
	OnConnect: ws.OnConnect(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes ws.ClientAttributes, r *http.Request) {
		ulog.Infof("Client connected %v", clientGuid)
	}),
	OnDisconnect: ws.OnDisconnect(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes ws.ClientAttributes, err error) {
		ulog.Infof("Client disonnected %v", clientGuid)
	}),
}
