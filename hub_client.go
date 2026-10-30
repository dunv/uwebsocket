package uwebsocket

import (
	"context"
	"net/http"
)

// Routing fields are immutable; active/snapshot and outbound channel operations
// are protected by clientLock. Stable entries let Send revalidate recipients
// without another registry lookup after payload construction.
type hubClient struct {
	client   WebSocketClient
	guid     string
	attrs    *ClientAttributes
	send     chan []byte
	ctx      context.Context
	active   bool
	snapshot *snapshotDelivery
	incoming chan ClientMessage
}

func (h *webSocketHub) enqueueLocked(client *hubClient, message []byte) {
	delivery := client.snapshot
	if delivery != nil {
		limits := delivery.limits
		if delivery.phase == snapshotBuffering {
			if len(delivery.pending) >= limits.MaxMessages || len(message) > limits.MaxBytes-delivery.bytes {
				h.failDeliveryLocked(client, "initialization buffer limit exceeded")
				return
			}
			delivery.pending = append(delivery.pending, message)
			delivery.bytes += len(message)
			return
		}
		if delivery.phase != snapshotLive {
			return
		}
		if len(message) > limits.MaxBytes-delivery.bytes || len(client.send) >= limits.MaxMessages {
			h.failDeliveryLocked(client, "live buffer limit exceeded")
			return
		}
	}
	select {
	case client.send <- message:
		if delivery != nil {
			delivery.bytes += len(message)
		}
	default:
		if delivery != nil {
			h.failDeliveryLocked(client, "live channel full")
		} else {
			h.discardedMessages++
			h.u.Log().Errorf("uwebsocket: buffer for client %s full, skipping msg", client.guid)
		}
	}
}

func (h *webSocketHub) messageDequeued(client WebSocketClient, size int) {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()
	if registered := h.clients[client.ClientGUID()]; registered != nil && registered.client == client && registered.snapshot != nil {
		registered.snapshot.bytes -= size
	}
}

func (h *webSocketHub) failDeliveryLocked(client *hubClient, reason string) {
	h.u.Log().Errorf("uwebsocket: disconnecting client:%s reason:%s", client.guid, reason)
	h.removeClientLocked(client)
}

func (h *webSocketHub) removeClientLocked(client *hubClient) {
	if !client.active {
		return
	}
	client.active = false
	client.snapshot = nil
	delete(h.clients, client.guid)
	client.client.Cancel()
	close(client.send)
}

func (h *webSocketHub) unregisterClient(client WebSocketClient) {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()
	if registered := h.clients[client.ClientGUID()]; registered != nil && registered.client == client {
		h.removeClientLocked(registered)
	}
}

func (h *webSocketHub) sendWelcome(client *hubClient, build func(WebSocketHub, string, *ClientAttributes, *http.Request, context.Context) ([][]byte, error)) {
	messages, err := build(h, client.guid, client.attrs, client.client.Request(), client.ctx)
	if err != nil {
		if client.ctx.Err() == nil {
			h.u.Log().Errorf("uwebsocket: could not generate welcome message: %v", err)
		}
		return
	}
	for _, message := range messages {
		h.clientLock.Lock()
		if !client.active || client.ctx.Err() != nil {
			h.clientLock.Unlock()
			return
		}
		h.enqueueLocked(client, message)
		h.clientLock.Unlock()
	}
}

func (h *webSocketHub) dispatchIncoming(client WebSocketClient, message []byte) bool {
	h.clientLock.Lock()
	defer h.clientLock.Unlock()
	registered := h.clients[client.ClientGUID()]
	if registered == nil || registered.client != client || registered.ctx.Err() != nil {
		return false
	}
	if registered.incoming == nil {
		return true
	}
	select {
	case registered.incoming <- ClientMessage{ClientGUID: registered.guid, Message: message}:
		return true
	default:
		h.failDeliveryLocked(registered, "incoming message queue full")
		return false
	}
}

func (h *webSocketHub) runIncoming(client *hubClient, handle func(WebSocketHub, string, *ClientAttributes, *http.Request, ClientMessage, context.Context)) {
	for {
		select {
		case <-client.ctx.Done():
			return
		case message := <-client.incoming:
			if client.ctx.Err() != nil {
				return
			}
			handle(h, client.guid, client.attrs, client.client.Request(), message, client.ctx)
		}
	}
}
