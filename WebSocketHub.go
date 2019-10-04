// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Adjusted by Daniel Unverricht

package uwebsocket

import (
	"fmt"
	"net/http"

	"github.com/dunv/uhttp"
)

type WebSocketHub struct {
	clients          map[*WebSocketClient]bool
	register         chan *WebSocketClient
	unregister       chan *WebSocketClient
	incomingMessages chan ClientMessage

	// if the user wants to receive messages, this channel needs to be not nil
	messageHandler *chan ClientMessage
}

func NewWebSocketHub(messagHandler *chan ClientMessage) *WebSocketHub {
	return &WebSocketHub{
		register:         make(chan *WebSocketClient),
		unregister:       make(chan *WebSocketClient),
		clients:          make(map[*WebSocketClient]bool),
		incomingMessages: make(chan ClientMessage),
		messageHandler:   messagHandler,
	}
}

func CreateHubAndRunInBackground(messagHandler *chan ClientMessage) *WebSocketHub {
	hub := NewWebSocketHub(messagHandler)
	go func() {
		hub.Run()
	}()
	return hub
}

func (h *WebSocketHub) SendWithFilter(filterFunc func(attrs ClientAttributes) bool, message []byte) error {
	for client, _ := range h.clients {
		if filterFunc(client.attributes) {
			select {
			case client.send <- message:
			default:
				h.unregister <- client
			}
		}
	}
	return nil
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case clientMessage := <-h.incomingMessages:
			if h.messageHandler != nil {
				*h.messageHandler <- clientMessage
			}
		}
	}
}

func (h *WebSocketHub) Handle(pattern string, handler *Handler) {
	if handler != nil {
		http.HandleFunc(pattern, handler.UhttpHandler.WsReady()(func(w http.ResponseWriter, r *http.Request) {
			var attributes ClientAttributes
			var err error
			if handler.ClientAttributes != nil {
				attributes, err = (*handler.ClientAttributes)(h, r)
				if err != nil {
					uhttp.RenderError(w, r, fmt.Errorf("could not get required attributes (%s)", err))
					return
				}
			}
			err = UpgradeConnection(h, attributes, w, r)
			if err != nil {
				uhttp.RenderError(w, r, fmt.Errorf("could not upgrade connection (%s)", err))
			}

			if handler.OnConnect != nil {
				(*handler.OnConnect)(h, attributes, r)
			}
		}))
	} else {
		http.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			config.CustomLog.LogIfError(UpgradeConnection(h, nil, w, r))
		})
	}
	config.CustomLog.Infof("Registered WS at %s", pattern)
}
