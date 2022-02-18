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

	textHub := ws.CreateHubAndRunInBackground(u, websocket.TextMessage, ctx)
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
		ws.WithWelcomeMessages(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request, ctx context.Context) ([][]byte, error) {
			go func() {
				<-ctx.Done()
				ulog.Infof("clientContext expired")
			}()
			return [][]byte{[]byte(fmt.Sprintf(`{"msg": "Welcome, %s"}`, clientGuid))}, nil
		}),
		ws.WithOnConnect(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request, ctx context.Context) {
			ulog.Infof("Client connected %v", clientGuid)
			go func() {
				<-ctx.Done()
				ulog.Infof("Client disconnected %v", clientGuid)
			}()
		}),
		ws.WithOnIncomingMessage(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request, msg ws.ClientMessage, ctx context.Context) {
			ulog.Infof("Received msg %s", msg.Message)
		}),
	))

	ulog.Fatal(u.ListenAndServe()) // need to investigate, why does this not work with localhost
}
