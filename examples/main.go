package main

import (
	"fmt"
	"net/http"

	"github.com/dunv/ulog"
	ws "github.com/dunv/uwebsocket"
	"github.com/google/uuid"
)

func main() {
	// implementation for an echo-server

	// can be tested with
	// websocat ws://localhost:8080/ws

	inboundMessages := make(chan ws.ClientMessage)
	wsHub := ws.CreateHubAndRunInBackground(&inboundMessages)

	go func() {
		for inboundMessage := range inboundMessages {
			ulog.LogIfError(wsHub.SendWithFilter(
				func(attrs ws.ClientAttributes) bool {
					return attrs.(map[string]interface{})["clientGuid"] == inboundMessage.Client.(map[string]interface{})["clientGuid"]
				},
				[]byte(fmt.Sprintf("response to %s", inboundMessage.Message)),
			))
		}
	}()

	wsHub.Handle("/ws", WsHandler)
	ulog.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
}

var WsHandler = &ws.Handler{
	ClientAttributes: ws.ClientAttributesFunc(func(hub *ws.WebSocketHub, r *http.Request) (ws.ClientAttributes, error) {
		clientAttributeMap := map[string]interface{}{"clientGuid": uuid.New().String()}
		return clientAttributeMap, nil
	}),
	OnConnect: ws.OnConnect(func(hub *ws.WebSocketHub, clientAttributes ws.ClientAttributes, r *http.Request) {
		ulog.Infof("Client connected %v", clientAttributes.(map[string]interface{})["clientGuid"])
	}),
	OnDisconnect: ws.OnDisconnect(func(hub *ws.WebSocketHub, clientAttributes ws.ClientAttributes, err error) {
		ulog.Infof("Client disonnected %v", clientAttributes.(map[string]interface{})["clientGuid"])
	}),
}
