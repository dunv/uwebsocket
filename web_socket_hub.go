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
	"github.com/gorilla/websocket"
)

var ErrClientNotFound = errors.New("client not found")

type WebSocketHub interface {
	Send(opts ...SendOption)
	Run()
	Handle(pattern string, handler Handler)
	CountClientsWithFilter(filterFunc func(clientGUID string, attrs *ClientAttributes) bool) int
}

type webSocketHub struct {
	clients    map[string]*hubClient
	register   chan WebSocketClient
	u          *uhttp.UHTTP
	clientLock sync.Mutex

	// Preserve callback serialization without holding the client registry lock.
	sendLock       sync.Mutex
	sendRecipients []*hubClient

	messageType int
	upgrader    websocket.Upgrader
	ctx         context.Context

	discardedMessages int64
}

func NewWebSocketHub(u *uhttp.UHTTP, messageType int, ctx context.Context) WebSocketHub {
	return &webSocketHub{
		register:    make(chan WebSocketClient),
		clients:     make(map[string]*hubClient),
		u:           u,
		messageType: messageType,
		ctx:         ctx,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

func CreateHubAndRunInBackground(u *uhttp.UHTTP, messageType int, ctx context.Context) WebSocketHub {
	hub := NewWebSocketHub(u, messageType, ctx)
	go hub.Run()
	return hub
}

func (h *webSocketHub) upgradeConnection(handler Handler, clientGuid string, clientAttributes *ClientAttributes, w http.ResponseWriter, r *http.Request, clientContext context.Context, clientContextCancel context.CancelFunc) error {
	upgrader := h.upgrader
	upgrader.CheckOrigin = h.checkOrigin
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("could not upgrade connection: %w", err)
	}
	capacity := 256
	if handler.wsOpts.initialSnapshot != nil {
		capacity = handler.wsOpts.initialSnapshot.limits.MaxMessages
	}
	client := &webSocketClient{
		hub:            h,
		conn:           conn,
		send:           make(chan []byte, capacity),
		clientGUID:     clientGuid,
		attributes:     clientAttributes,
		connectRequest: r,
		handler:        handler,
		ctx:            clientContext,
		ctxCancel:      clientContextCancel,
	}
	select {
	case h.register <- client:
		return nil
	case <-clientContext.Done():
		_ = conn.Close() // Best-effort teardown after cancellation.
		return clientContext.Err()
	}
}

func (h *webSocketHub) CountClientsWithFilter(filterFunc func(clientGUID string, attrs *ClientAttributes) bool) int {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()
	count := 0
	for _, client := range h.clients {
		if filterFunc(client.guid, client.attrs) {
			count++
		}
	}
	return count
}

// Send selects recipients, then builds one shared message outside the client
// registry lock. Calls and their builders are serialized; concurrent callers have
// no defined acquisition order. Builders must return immutable bytes.
// Legacy clients drop on overflow; WithInitialSnapshot clients disconnect instead.
// Clients awaiting their explicit snapshot boundary are not live recipients.
func (h *webSocketHub) Send(opts ...SendOption) {
	h.sendLock.Lock()
	defer h.sendLock.Unlock()
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
	recipients := h.sendRecipients[:0]
	for _, client := range h.clients {
		if client.snapshot != nil && client.snapshot.phase == snapshotWaiting {
			continue
		}
		if sendOpts.filterFn(client.guid, client.attrs) {
			recipients = append(recipients, client)
		}
	}
	h.clientLock.Unlock()
	defer func() {
		clear(recipients)
		h.sendRecipients = recipients[:0]
	}()
	if len(recipients) == 0 {
		return
	}
	message, err := sendOpts.messageFn()
	if err != nil {
		h.u.Log().Errorf("uwebsocket: err generating msg: %v", err)
	}
	h.clientLock.Lock()
	defer h.clientLock.Unlock()
	for _, client := range recipients {
		if !client.active {
			continue
		}
		if err != nil {
			if client.snapshot != nil {
				h.removeClientLocked(client)
			}
			continue
		}
		h.enqueueLocked(client, message)
	}
}

func (h *webSocketHub) Run() {
	for {
		select {
		case <-h.ctx.Done():
			h.clientLock.Lock()
			for _, client := range h.clients {
				h.removeClientLocked(client)
			}
			h.clientLock.Unlock()
			return
		case client := <-h.register:
			h.registerClient(client)
		}
	}
}

func (h *webSocketHub) registerClient(client WebSocketClient) {
	opts := client.Handler().wsOpts
	registered := &hubClient{
		client: client, guid: client.ClientGUID(), attrs: client.Attributes(),
		send: client.SendChan(), ctx: client.Ctx(), active: true,
	}
	if opts.initialSnapshot != nil {
		registered.snapshot = &snapshotDelivery{limits: opts.initialSnapshot.limits}
	}
	if opts.onIncomingMessage != nil {
		registered.incoming = make(chan ClientMessage, opts.incomingMessageLimit)
	}
	h.clientLock.Lock()
	h.clients[registered.guid] = registered
	// Install all routing state before starting the socket reader.
	client.Run(registered.ctx)
	h.clientLock.Unlock()
	if opts.onIncomingMessage != nil {
		go h.runIncoming(registered, *opts.onIncomingMessage)
	}
	if opts.initialSnapshot != nil {
		go h.startSnapshot(registered, opts.initialSnapshot)
	} else if opts.welcomeMessages != nil {
		go h.sendWelcome(registered, *opts.welcomeMessages)
	}
}

func (h *webSocketHub) Handle(pattern string, handler Handler) {
	h.u.ServeMux().Handle(pattern, handler.wsOpts.uhttpHandler.WsReady(h.u)(func(w http.ResponseWriter, r *http.Request) {
		clientGuid := uuid.New().String()
		attributes := NewClientAttributes()
		var err error
		if handler.wsOpts.clientAttributes != nil {
			attributes, err = (*handler.wsOpts.clientAttributes)(h, r)
			if err != nil {
				h.u.RenderError(w, r, fmt.Errorf("could not get required attributes: %w", err))
				return
			}
		}
		clientContext, cancel := context.WithCancel(h.ctx)
		err = h.upgradeConnection(handler, clientGuid, attributes, w, r, clientContext, cancel)
		if err != nil {
			// Gorilla already wrote upgrade errors, or the connection was hijacked.
			// Never attempt another HTTP response after either outcome.
			h.u.Log().Errorf("uwebsocket: connection upgrade failed: %v", err)
			cancel()
			return
		}
		if handler.wsOpts.onConnect != nil {
			(*handler.wsOpts.onConnect)(h, clientGuid, attributes, r, clientContext)
		}
	}))
	h.u.Log().Infof("Registered WS at %s", pattern)
}
