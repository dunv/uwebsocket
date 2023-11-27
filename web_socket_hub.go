// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Adjusted by Daniel Unverricht

package uwebsocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/dunv/uhttp"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var ErrClientNotFound = errors.New("client not found")

type WebSocketHub struct {
	clients          map[string]WebSocketClient
	register         chan WebSocketClient
	unregister       chan WebSocketClient
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

	// how many messages were discarded because the client-buffer was full
	discardedMessages int64
}

func NewWebSocketHub(u *uhttp.UHTTP, messageType int, ctx context.Context) *WebSocketHub {
	return &WebSocketHub{
		register:         make(chan WebSocketClient),
		unregister:       make(chan WebSocketClient),
		clients:          make(map[string]WebSocketClient),
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
	client := &webSocketClient{
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
		if filterFunc(client.ClientGUID(), client.Attributes()) {
			count++
		}
	}
	return count
}

func (h *WebSocketHub) Send(ctx context.Context, opts ...SendOption) {
	sendOpts := &sendOptions{
		filterFn: func(clientGUID string, attrs *ClientAttributes) bool { return true },
	}
	for _, opt := range opts {
		opt(sendOpts)
	}
	if sendOpts.messageFn == nil {
		h.u.Log().Errorf("uwebsocket: err no message or messageFn specified")
		return
	}

	h.clientLock.Lock()
	defer h.clientLock.Unlock()

	// Cache generated message, this way the message-callback
	// - is only called if there is at least one filter-match
	// - is only called once for all clients
	var generatedMessage []byte = nil
	var generatedErr error

	for i := range h.clients {
		client := h.clients[i]
		if sendOpts.filterFn(client.ClientGUID(), client.Attributes()) {
			// if message generation already failed once, the error was logged and can be skipped this time around
			if generatedErr != nil {
				continue
			}

			// message was never generated -> do it here
			if generatedMessage == nil {
				generatedMessage, generatedErr = sendOpts.messageFn()
				if generatedErr != nil {
					h.u.Log().Errorf("uwebsocket: err generating msg: %w", generatedErr)
					generatedMessage = []byte{}
					continue
				}
			}

			// send synchronously here, as messages are buffered in the client
			select {
			case client.SendChan() <- generatedMessage:
			default:
				h.discardedMessages++
				h.u.Log().Errorf("uwebsocket: buffer for client %s full, skipping msg", client.ClientGUID())
			}
		}
	}
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case <-h.ctx.Done():
			h.clientLock.Lock()
			for _, client := range h.clients {
				delete(h.clients, client.ClientGUID())
				close(client.SendChan())
				client.Cancel()
			}
			h.clientLock.Unlock()
			return
		case client := <-h.register:
			h.clientLock.Lock()
			h.clients[client.ClientGUID()] = client
			client.Run(h.ctx)
			h.clientLock.Unlock()
			if client.Handler().wsOpts.welcomeMessages != nil {
				if welcomeMessages, err := (*client.Handler().wsOpts.welcomeMessages)(h, client.ClientGUID(), client.Attributes(), client.Request(), client.Ctx()); err == nil {
					for _, msg := range welcomeMessages {
						client.SendChan() <- msg
					}
				} else {
					h.u.Log().Errorf("Could not generate welcomeMessage %v", err)
				}
			}
		case client := <-h.unregister:
			h.clientLock.Lock()
			if _, ok := h.clients[client.ClientGUID()]; ok {
				delete(h.clients, client.ClientGUID())
				close(client.SendChan())
				client.Cancel()
			}
			h.clientLock.Unlock()
		case clientMessage := <-h.incomingMessages:
			h.messageHandlersLock.Lock()
			for clientGUID, handler := range h.messageHandlers {
				if clientMessage.ClientGUID == clientGUID {
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
