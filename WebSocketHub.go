// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Adjusted by Daniel Unverricht

package uwebsocket

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/dunv/uhttp"
)

type WebSocketHub struct {
	clients          map[*WebSocketClient]bool
	register         chan *WebSocketClient
	unregister       chan *WebSocketClient
	incomingMessages chan ClientMessage

	// if the user wants to receive messages, this channel needs to be not nil
	messageHandler *chan ClientMessage

	u *uhttp.UHTTP

	// lock list
	clientLock *sync.Mutex
}

func NewWebSocketHub(u *uhttp.UHTTP, messagHandler *chan ClientMessage) *WebSocketHub {
	return &WebSocketHub{
		register:         make(chan *WebSocketClient),
		unregister:       make(chan *WebSocketClient),
		clients:          make(map[*WebSocketClient]bool),
		incomingMessages: make(chan ClientMessage),
		messageHandler:   messagHandler,
		u:                u,
		clientLock:       &sync.Mutex{},
	}
}

func CreateHubAndRunInBackground(u *uhttp.UHTTP, messageHandler *chan ClientMessage) *WebSocketHub {
	hub := NewWebSocketHub(u, messageHandler)
	go func() {
		hub.Run()
	}()
	return hub
}

func (h *WebSocketHub) SendWithFilter(filterFunc func(attrs ClientAttributes) bool, message []byte) error {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for client := range h.clients {
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
			h.clientLock.Lock()
			h.clients[client] = true
			h.clientLock.Unlock()
		case client := <-h.unregister:
			h.clientLock.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.clientLock.Unlock()
		case clientMessage := <-h.incomingMessages:
			if h.messageHandler != nil {
				*h.messageHandler <- clientMessage
			}
		}
	}
}

func (h *WebSocketHub) Handle(pattern string, handler *Handler) {
	// Add all middlewares, if handler is defined
	if handler != nil {
		h.u.ServeMux().Handle(pattern, handler.UhttpHandler.WsReady(h.u)(func(w http.ResponseWriter, r *http.Request) {
			var attributes ClientAttributes
			var err error
			if handler.ClientAttributes != nil {
				attributes, err = (*handler.ClientAttributes)(h, r)
				if err != nil {
					h.u.RenderError(w, r, fmt.Errorf("could not get required attributes (%s)", err))
					return
				}
			}
			err = UpgradeConnection(h, handler, attributes, w, r)
			if err != nil {
				h.u.RenderError(w, r, fmt.Errorf("could not upgrade connection (%s)", err))
				return
			}

			if handler.OnConnect != nil {
				(*handler.OnConnect)(h, attributes, r)
			}
		}))
	} else {
		h.u.ServeMux().HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			config.CustomLog.LogIfError(UpgradeConnection(h, nil, nil, w, r))
		})
	}
	config.CustomLog.Infof("Registered WS at %s", pattern)
}
