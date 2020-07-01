// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Adjusted by Daniel Unverricht

package uwebsocket

import (
	"bytes"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client is a middleman between the websocket connection and the hub.
type WebSocketClient struct {
	hub *WebSocketHub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	attributes ClientAttributes

	handler *Handler
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	err := c.conn.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		if c.handler != nil && c.handler.OnError != nil {
			(*c.handler.OnError)(fmt.Errorf("Could not SetReadDeadline on connection (%v)", err))
		} else {
			config.CustomLog.Errorf("Could not SetReadDeadline on connection (%v)", err)
		}
	}
	c.conn.SetPongHandler(func(string) error {
		err = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		if err != nil {
			if c.handler != nil && c.handler.OnError != nil {
				(*c.handler.OnError)(fmt.Errorf("Could not SetPongHandler on connection (%v)", err))
			} else {
				config.CustomLog.Errorf("Could not SetPongHandler on connection (%v)", err)
			}
		}
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if c.handler != nil && c.handler.OnError != nil {
					(*c.handler.OnError)(fmt.Errorf("UnexpectedCloseError on connection (%v)", err))
				} else {
					config.CustomLog.Errorf("UnexpectedCloseError on connection (%v)", err)
				}
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))

		// support client side ping-pong via text-message
		if bytes.Equal(message, []byte("PING")) {
			c.send <- []byte(`PONG`)
			continue
		}

		c.hub.incomingMessages <- ClientMessage{
			Client:  c.attributes,
			Message: message,
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			err := c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err != nil {
				if c.handler != nil && c.handler.OnError != nil {
					(*c.handler.OnError)(fmt.Errorf("Could not SetWriteDeadline on connection (%v)", err))
				} else {
					config.CustomLog.Errorf("Could not SetWriteDeadline on connection (%v)", err)
				}
			}

			if !ok {
				// The hub closed the channel.
				err = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				if err != nil {
					if c.handler != nil && c.handler.OnDisconnect != nil {
						(*c.handler.OnDisconnect)(c.hub, c.attributes, fmt.Errorf("WebsocketConnection closed (%v)", err))
					} else {
						config.CustomLog.Infof("WebsocketConnection closed (%v)", err)
					}
				}
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, err = w.Write(message)
			if err != nil {
				if c.handler != nil && c.handler.OnError != nil {
					(*c.handler.OnError)(fmt.Errorf("Could not Write on writer (%v)", err))
				} else {
					config.CustomLog.Errorf("Could not Write on writer (%s)", err)
				}
			}

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				_, err := w.Write(newline)
				if err != nil {
					if c.handler != nil && c.handler.OnError != nil {
						(*c.handler.OnError)(fmt.Errorf("Could not Write on writer (%v)", err))
					} else {
						config.CustomLog.Errorf("Could not Write on writer (%s)", err)
					}
				}
				_, err = w.Write(<-c.send)
				if err != nil {
					if c.handler != nil && c.handler.OnError != nil {
						(*c.handler.OnError)(fmt.Errorf("Could not Write on writer (%v)", err))
					} else {
						config.CustomLog.Errorf("Could not Write on writer (%s)", err)
					}
				}
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			err := c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err != nil {
				config.CustomLog.Errorf("Could not SetWriteDeadline on connection (%s)", err)
				if c.handler != nil && c.handler.OnError != nil {
					(*c.handler.OnError)(fmt.Errorf("Could not SetWriteDeadline on connection (%v)", err))
				} else {
					config.CustomLog.Errorf("Could not SetWriteDeadline on connection (%s)", err)
				}
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if c.handler != nil && c.handler.OnDisconnect != nil {
					(*c.handler.OnDisconnect)(c.hub, c.attributes, fmt.Errorf("WebsocketConnection closed, could not send ping (%v)", err))
				} else {
					config.CustomLog.Infof("WebsocketConnection closed, could not send ping (%v)", err)
				}
				return
			}
		}
	}
}
