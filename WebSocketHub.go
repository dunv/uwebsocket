// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Adjusted by Daniel Unverricht

package uwebsocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/dunv/uhttp"
	"github.com/google/uuid"
)

var ErrClientNotFound = errors.New("client not found")

type WebSocketHub struct {
	clients          map[string]*WebSocketClient
	register         chan *WebSocketClient
	unregister       chan *WebSocketClient
	incomingMessages chan ClientMessage

	// if the user wants to receive messages, this channel needs to be not nil
	messageHandler *chan ClientMessage

	u *uhttp.UHTTP

	// lock list
	clientLock *sync.Mutex

	messageType int

	// this context can cancel the run-routine
	ctx context.Context
}

func NewWebSocketHub(u *uhttp.UHTTP, messagHandler *chan ClientMessage, messageType int, ctx context.Context) *WebSocketHub {
	return &WebSocketHub{
		register:         make(chan *WebSocketClient),
		unregister:       make(chan *WebSocketClient),
		clients:          make(map[string]*WebSocketClient),
		incomingMessages: make(chan ClientMessage),
		messageHandler:   messagHandler,
		u:                u,
		clientLock:       &sync.Mutex{},
		messageType:      messageType,
		ctx:              ctx,
	}
}

func CreateHubAndRunInBackground(u *uhttp.UHTTP, messageHandler *chan ClientMessage, messageType int, ctx context.Context) *WebSocketHub {
	hub := NewWebSocketHub(u, messageHandler, messageType, ctx)
	go func() {
		hub.Run()
	}()
	return hub
}

func (h *WebSocketHub) SendWithFilterSync(filterFunc func(clientGUID string, attrs *ClientAttributes) bool, message []byte, ctx context.Context) error {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for _, client := range h.clients {
		if filterFunc(client.clientGUID, client.attributes) {
			select {
			case client.send <- message:
			case <-ctx.Done():
				return context.DeadlineExceeded
			}
		}
	}
	return nil
}

func (h *WebSocketHub) SendToAllWithFlagSync(flag string, message []byte, ctx context.Context) error {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for _, client := range h.clients {
		if client.attributes.IsFlagSet(flag) {
			select {
			case client.send <- message:
			case <-ctx.Done():
				return context.DeadlineExceeded
			}
		}
	}
	return nil
}

func (h *WebSocketHub) SendToAllWithMatchSync(key string, value string, message []byte, ctx context.Context) error {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for _, client := range h.clients {
		if client.attributes.HasMatch(key, value) {
			select {
			case client.send <- message:
			case <-ctx.Done():
				return context.DeadlineExceeded
			}
		}
	}
	return nil
}

func (h *WebSocketHub) SendToClientSync(clientGUID string, message []byte, ctx context.Context) error {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	if client, ok := h.clients[clientGUID]; ok {
		select {
		case client.send <- message:
			return nil
		case <-ctx.Done():
			return context.DeadlineExceeded
		}
	}

	return ErrClientNotFound
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case <-h.ctx.Done():
			h.clientLock.Lock()
			for _, client := range h.clients {
				delete(h.clients, client.clientGUID)
				close(client.send)
			}
			h.clientLock.Unlock()
			return
		case client := <-h.register:
			h.clientLock.Lock()
			h.clients[client.clientGUID] = client
			go client.writePump(h.ctx)
			go client.readPump(h.ctx)
			h.clientLock.Unlock()
			if client.handler != nil && client.handler.WelcomeMessages != nil {
				if welcomeMessages, err := (*client.handler.WelcomeMessages)(h, client.clientGUID, client.attributes, client.connectRequest); err == nil {
					for _, msg := range welcomeMessages {
						client.send <- msg
					}
				} else {
					config.CustomLog.Errorf("Could not generate welcomeMessage %v", err)
				}
			}
		case client := <-h.unregister:
			h.clientLock.Lock()
			if _, ok := h.clients[client.clientGUID]; ok {
				delete(h.clients, client.clientGUID)
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
			clientGuid := uuid.New().String()
			attributes := NewClientAttributes()
			var err error
			if handler.ClientAttributes != nil {
				attributes, err = (*handler.ClientAttributes)(h, r)
				if err != nil {
					h.u.RenderError(w, r, fmt.Errorf("could not get required attributes (%s)", err))
					return
				}
			}
			err = UpgradeConnection(h, handler, clientGuid, attributes, w, r)
			if err != nil {
				h.u.RenderError(w, r, fmt.Errorf("could not upgrade connection (%s)", err))
				return
			}

			if handler.OnConnect != nil {
				(*handler.OnConnect)(h, clientGuid, attributes, r)
			}
		}))
	} else {
		h.u.ServeMux().HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			config.CustomLog.LogIfError(UpgradeConnection(h, nil, uuid.New().String(), nil, w, r))
		})
	}
	config.CustomLog.Infof("Registered WS at %s", pattern)
}
