// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Adjusted by Daniel Unverricht

package uwebsocket

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/dunv/uhttp"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var ErrClientNotFound = errors.New("client not found")

type WebSocketHub struct {
	clients          map[string]*WebSocketClient
	register         chan *WebSocketClient
	unregister       chan *WebSocketClient
	incomingMessages chan ClientMessage

	// map[clientGUID]chan ClientMessage
	messageHandlers     map[string]func(ClientMessage)
	messageHandlersLock *sync.Mutex

	u *uhttp.UHTTP

	// lock list
	clientLock *sync.Mutex

	// websocket message-types (text or bytes for sending and receiving)
	messageType int

	// hold a configured upgrader
	upgrader websocket.Upgrader

	// this context can cancel the run-routine
	ctx context.Context
}

func NewWebSocketHub(u *uhttp.UHTTP, messageType int, ctx context.Context) *WebSocketHub {
	return &WebSocketHub{
		register:         make(chan *WebSocketClient),
		unregister:       make(chan *WebSocketClient),
		clients:          make(map[string]*WebSocketClient),
		incomingMessages: make(chan ClientMessage),
		u:                u,
		clientLock:       &sync.Mutex{},
		messageType:      messageType,
		ctx:              ctx,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		messageHandlers:     make(map[string]func(ClientMessage)),
		messageHandlersLock: &sync.Mutex{},
	}
}

func CreateHubAndRunInBackground(u *uhttp.UHTTP, messageType int, ctx context.Context) *WebSocketHub {
	hub := NewWebSocketHub(u, messageType, ctx)
	go func() {
		hub.Run()
	}()
	return hub
}

func (h *WebSocketHub) upgradeConnection(handler Handler, clientGuid string, clientAttributes *ClientAttributes, w http.ResponseWriter, r *http.Request, clientContext context.Context, clientContextCancel context.CancelFunc) error {
	h.upgrader.CheckOrigin = func(r *http.Request) bool {
		if h.u.CORS() == "*" {
			return true
		}
		for _, host := range strings.Split(h.u.CORS(), ",") {
			if host == r.Host {
				return true
			}
		}
		return false
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("Could not Upgrade connection (%s)", err)
	}
	client := &WebSocketClient{
		hub:            h,
		conn:           conn,
		send:           make(chan []byte, 256),
		clientGUID:     clientGuid,
		attributes:     clientAttributes,
		connectRequest: r,
		handler:        handler,
		ctx:            clientContext,
		ctxCancel:      clientContextCancel,
	}
	client.hub.register <- client
	return nil
}

func (h *WebSocketHub) CountClientsWithFilter(filterFunc func(clientGUID string, attrs *ClientAttributes) bool) int {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	count := 0
	for i := range h.clients {
		client := h.clients[i]
		if filterFunc(client.clientGUID, client.attributes) {
			count++
		}
	}
	return count
}

func (h *WebSocketHub) Send(ctx context.Context, opts ...SendOption) {
	sendOpts := &sendOptions{
		filterFn: func(clientGUID string, attrs *ClientAttributes) bool { return true },
		async:    false,
	}
	for _, opt := range opts {
		opt(sendOpts)
	}
	if sendOpts.messageFn == nil {
		slog.Error("uwebsocket: err no message or messageFn specified")
		return
	}

	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for i := range h.clients {
		client := h.clients[i]
		if sendOpts.filterFn(client.clientGUID, client.attributes) {
			msg, err := sendOpts.messageFn(client.clientGUID, client.attributes)
			if err != nil {
				slog.Error("uwebsocket: err generating msg: %w", err)
				continue
			}

			if sendOpts.async {
				go func() {
					select {
					case client.send <- msg:
					case <-ctx.Done():
					}
				}()
				continue
			}

			select {
			case client.send <- msg:
			case <-ctx.Done():
			}
		}
	}
}

func (h *WebSocketHub) SendWithFilterSync(filterFunc func(clientGUID string, attrs *ClientAttributes) bool, message []byte, ctx context.Context) error {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for i := range h.clients {
		client := h.clients[i]

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

func (h *WebSocketHub) SendWithFilterAsync(filterFunc func(clientGUID string, attrs *ClientAttributes) bool, message []byte, ctx context.Context) {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for i := range h.clients {
		client := h.clients[i]

		if filterFunc(client.clientGUID, client.attributes) {
			go func(client *WebSocketClient) {
				select {
				case client.send <- message:
				case <-ctx.Done():
				}
			}(client)
		}
	}
}

func (h *WebSocketHub) SendToAllWithFlagSync(flag string, message []byte, ctx context.Context) error {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for i := range h.clients {
		client := h.clients[i]

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

func (h *WebSocketHub) SendToAllWithFlagAsync(flag string, message []byte, ctx context.Context) {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for i := range h.clients {
		client := h.clients[i]

		if client.attributes.IsFlagSet(flag) {
			go func(client *WebSocketClient) {
				select {
				case client.send <- message:
				case <-ctx.Done():
				}
			}(client)
		}
	}
}

func (h *WebSocketHub) SendToAllWithMatchSync(key string, value string, message []byte, ctx context.Context) error {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for i := range h.clients {
		client := h.clients[i]

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

func (h *WebSocketHub) SendToAllWithMatchAsync(key string, value string, message []byte, ctx context.Context) {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	for i := range h.clients {
		client := h.clients[i]

		if client.attributes.HasMatch(key, value) {
			go func(client *WebSocketClient) {
				select {
				case client.send <- message:
				case <-ctx.Done():
				}
			}(client)
		}
	}
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
				client.ctxCancel()
			}
			h.clientLock.Unlock()
			return
		case client := <-h.register:
			h.clientLock.Lock()
			h.clients[client.clientGUID] = client
			go client.writePump(h.ctx)
			go client.readPump(h.ctx)
			h.clientLock.Unlock()
			if client.handler.wsOpts.welcomeMessages != nil {
				if welcomeMessages, err := (*client.handler.wsOpts.welcomeMessages)(h, client.clientGUID, client.attributes, client.connectRequest, client.ctx); err == nil {
					for _, msg := range welcomeMessages {
						client.send <- msg
					}
				} else {
					h.u.Log().Errorf("Could not generate welcomeMessage %v", err)
				}
			}
		case client := <-h.unregister:
			h.clientLock.Lock()
			if _, ok := h.clients[client.clientGUID]; ok {
				delete(h.clients, client.clientGUID)
				close(client.send)
				client.ctxCancel()
			}
			h.clientLock.Unlock()
		case clientMessage := <-h.incomingMessages:
			h.messageHandlersLock.Lock()
			fmt.Println("going through")
			for clientGUID, handler := range h.messageHandlers {
				fmt.Println("clientGUID", clientGUID)
				if clientMessage.ClientGUID == clientGUID {
					fmt.Println("match")
					handler(clientMessage)
				}
			}
			h.messageHandlersLock.Unlock()
		}
	}
}

func (h *WebSocketHub) Handle(pattern string, handler Handler) {
	h.u.ServeMux().Handle(pattern, handler.wsOpts.uhttpHandler.WsReady(h.u)(func(w http.ResponseWriter, r *http.Request) {
		clientGuid := uuid.New().String()
		attributes := NewClientAttributes()
		var err error
		if handler.wsOpts.clientAttributes != nil {
			attributes, err = (*handler.wsOpts.clientAttributes)(h, r)
			if err != nil {
				h.u.RenderError(w, r, fmt.Errorf("could not get required attributes (%s)", err))
				return
			}
		}
		clientContext, cancel := context.WithCancel(h.ctx)
		err = h.upgradeConnection(handler, clientGuid, attributes, w, r, clientContext, cancel)
		if err != nil {
			h.u.RenderError(w, r, fmt.Errorf("could not upgrade connection (%s)", err))
			cancel()
			return
		}

		if handler.wsOpts.onIncomingMessage != nil {
			fmt.Println("registered")
			h.messageHandlersLock.Lock()
			h.messageHandlers[clientGuid] = func(msg ClientMessage) {
				(*handler.wsOpts.onIncomingMessage)(h, clientGuid, attributes, r, msg, clientContext)
			}
			h.messageHandlersLock.Unlock()
		}

		if handler.wsOpts.onConnect != nil {
			(*handler.wsOpts.onConnect)(h, clientGuid, attributes, r, clientContext)
		}
	}))

	h.u.Log().Infof("Registered WS at %s", pattern)
}
