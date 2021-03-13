package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dunv/uhttp"
	"github.com/dunv/ulog"
	ws "github.com/dunv/uwebsocket"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func main() {
	// implementation for an echo-server

	// command-line: websocat ws://localhost:8080/wsText
	// javascript:   npm start

	u := uhttp.NewUHTTP(uhttp.WithAddress("0.0.0.0:8080"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	textInboundMessages := make(chan ws.ClientMessage)
	textHub := ws.CreateHubAndRunInBackground(u, &textInboundMessages, websocket.TextMessage, ctx)
	go func() {
		for inboundMessage := range textInboundMessages {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			ulog.LogIfError(textHub.SendToClientSync(
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
			ulog.Infof("scanned %s", input)

			marshaled, err := json.Marshal(map[string]string{"received": input})
			if err != nil {
				ulog.Errorf("could not marshal (%s)", err)
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			ulog.LogIfError(textHub.SendWithFilterSync(func(clientGUID string, attrs *ws.ClientAttributes) bool {
				return true
			}, marshaled, ctx))
			cancel()
		}
	}()
	textHub.Handle("/wsText", ws.NewHandler(
		ws.WithClientAttributes(func(hub *ws.WebSocketHub, r *http.Request) (*ws.ClientAttributes, error) {
			return ws.NewClientAttributes().SetString("testGuid", uuid.New().String()), nil
		}),
		ws.WithWelcomeMessages(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request) ([][]byte, error) {
			return [][]byte{[]byte(fmt.Sprintf(`{"msg": "Welcome, %s"}`, clientGuid))}, nil
		}),
		ws.WithOnConnect(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request) {
			ulog.Infof("Client connected %v", clientGuid)
		}),
		ws.WithOnDisconnect(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request, err error) {
			ulog.Infof("Client disonnected %v", clientGuid)
		}),
	))

	ulog.Fatal(u.ListenAndServe()) // need to investigate, why does this not work with localhost
}
