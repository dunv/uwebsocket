package uwebsocket

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

type WebSocketClientMock struct {
	t            *testing.T
	clientGUID   string
	attributes   *ClientAttributes
	sendChan     chan []byte
	ctx          context.Context
	handler      Handler
	r            *http.Request
	calledCancel bool
	calledRun    bool
}

func NewWebSocketClientMock(t *testing.T, ctx context.Context, guid string, attrs *ClientAttributes) *WebSocketClientMock {
	return &WebSocketClientMock{
		t:          t,
		clientGUID: guid,
		attributes: attrs,
		sendChan:   make(chan []byte, 3),
		ctx:        ctx,
	}
}

func (c *WebSocketClientMock) ClientGUID() string            { return c.clientGUID }
func (c *WebSocketClientMock) Attributes() *ClientAttributes { return c.attributes }
func (c *WebSocketClientMock) SendChan() chan []byte         { return c.sendChan }
func (c *WebSocketClientMock) Ctx() context.Context          { return c.ctx }
func (c *WebSocketClientMock) Cancel()                       { c.calledCancel = true }
func (c *WebSocketClientMock) Handler() Handler              { return c.handler }
func (c *WebSocketClientMock) Request() *http.Request        { return c.r }
func (c *WebSocketClientMock) Run(ctx context.Context)       { c.calledRun = true }

func (c *WebSocketClientMock) readOne() ([]byte, error) {
	select {
	case msg := <-c.sendChan:
		return msg, nil
	default:
		return nil, fmt.Errorf("buffer empty")
	}
}
