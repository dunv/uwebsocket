// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Adjusted by Daniel Unverricht

package uwebsocket

import (
	"bytes"
	"context"
	"net/http"
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

type WebSocketClient interface {
	ClientGUID() string
	Attributes() *ClientAttributes
	SendChan() chan []byte
	Ctx() context.Context
	Cancel()
	Handler() Handler
	Request() *http.Request
	Run(ctx context.Context)
}

// Client is a middleman between the websocket connection and the hub.
type webSocketClient struct {
	hub *WebSocketHub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	// ClientAttributes
	connectRequest *http.Request
	clientGUID     string
	attributes     *ClientAttributes

	handler   Handler
	ctx       context.Context
	ctxCancel context.CancelFunc
}

func (c *webSocketClient) ClientGUID() string {
	return c.clientGUID
}

func (c *webSocketClient) Attributes() *ClientAttributes {
	return c.attributes
}

func (c *webSocketClient) SendChan() chan []byte {
	return c.send
}

func (c *webSocketClient) Ctx() context.Context {
	return c.ctx
}

func (c *webSocketClient) Cancel() {
	c.ctxCancel()
}

func (c *webSocketClient) Handler() Handler {
	return c.handler
}

func (c *webSocketClient) Request() *http.Request {
	return c.connectRequest
}

func (c *webSocketClient) Run(ctx context.Context) {
	go c.writePump(ctx)
	go c.readPump(ctx)
}

func (c *webSocketClient) handleError(err error) {
	if c.handler.wsOpts.onError != nil {
		(*c.handler.wsOpts.onError)(c.hub, c.clientGUID, c.attributes, c.connectRequest, err, c.ctx)
	} else {
		c.hub.u.Log().Errorf("%s", err)
	}
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *webSocketClient) readPump(ctx context.Context) {
	readContext, cancel := context.WithCancel(ctx)

	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
		cancel()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	err := c.conn.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		c.handleError(err)
		return
	}

	c.conn.SetPongHandler(func(input string) error {
		err = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		if err != nil {
			cancel()
			c.handleError(err)
		}
		return nil
	})

	for {
		if err := readContext.Err(); err != nil {
			c.handleError(err)
			return
		}

		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// some "unexpected" messages are actually ok
				return
			}
			c.handleError(err)
			return
		}
		message = bytes.TrimSpace(bytes.ReplaceAll(message, newline, space))

		// support client side ping-pong via text-message
		if bytes.Equal(message, []byte("PING")) {
			c.send <- []byte(`PONG`)
			continue
		}

		c.hub.incomingMessages <- ClientMessage{
			ClientGUID: c.clientGUID,
			Message:    message,
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *webSocketClient) writePump(ctx context.Context) {
	writeContext, cancel := context.WithCancel(ctx)

	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		cancel()
	}()

	for {
		if err := writeContext.Err(); err != nil {
			c.handleError(err)
			break
		}

		select {
		case message, ok := <-c.send:
			err := c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err != nil {
				c.handleError(err)
				return
			}

			if !ok {
				// The hub closed the channel.
				err = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				if err != nil {
					c.handleError(err)
				}
				return
			}

			w, err := c.conn.NextWriter(c.hub.messageType)
			if err != nil {
				c.handleError(err)
				return
			}
			_, err = w.Write(message)
			if err != nil {
				c.handleError(err)
				return
			}

			if err := w.Close(); err != nil {
				c.handleError(err)
				return
			}
		case <-ticker.C:
			err := c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err != nil {
				c.handleError(err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.handleError(err)
				return
			}
		}
	}
}
