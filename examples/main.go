package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dunv/uhttp"
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
				u.Log().Errorf("could not scan (%s)", err)
				continue
			}
			u.Log().Infof("scanned %s", input)

			marshaled, err := json.Marshal(map[string]string{"received": input})
			if err != nil {
				u.Log().Errorf("could not marshal (%s)", err)
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := textHub.SendWithFilterSync(func(clientGUID string, attrs *ws.ClientAttributes) bool {
				return true
			}, marshaled, ctx); err != nil {
				u.Log().Errorf("%s", err)
			}

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
				u.Log().Infof("clientContext expired")
			}()
			return [][]byte{[]byte(fmt.Sprintf(`{"msg": "Welcome, %s"}`, clientGuid))}, nil
		}),
		ws.WithOnConnect(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request, ctx context.Context) {
			u.Log().Infof("Client connected %v", clientGuid)
			go func() {
				<-ctx.Done()
				u.Log().Infof("Client disconnected %v", clientGuid)
			}()
		}),
		ws.WithOnIncomingMessage(func(hub *ws.WebSocketHub, clientGuid string, clientAttributes *ws.ClientAttributes, r *http.Request, msg ws.ClientMessage, ctx context.Context) {
			u.Log().Infof("Received msg %s", msg.Message)
		}),
	))

	if err := u.ListenAndServe(); err != nil { // need to investigate, why does this not work with localhost
		u.Log().Errorf("%s", err)
	}
}
