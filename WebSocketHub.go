// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Adjusted by Daniel Unverricht

package uwebsocket

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type WebSocketHub struct {
	// Registered clients.
	clients map[*WebSocketClient]bool

	// Inbound messages from the clients.
	Broadcast chan []byte

	// Register requests from the clients.
	register chan *WebSocketClient

	// Unregister requests from clients.
	unregister chan *WebSocketClient
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		Broadcast:  make(chan []byte),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		clients:    make(map[*WebSocketClient]bool),
	}
}

func (h *WebSocketHub) SendWithFilter(filterFunc func(attrs map[string]interface{}) bool, message []byte) error {

	for client, _ := range h.clients {
		if filterFunc(client.Attributes) {
			client.send <- message
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
		case message := <-h.Broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}
