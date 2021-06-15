package uwebsocket

import (
	"context"
	"net/http"

	"github.com/dunv/uhttp"
)

type HandlerOption interface {
	apply(*handlerOptions)
}

type handlerOptions struct {
	uhttpHandler     uhttp.Handler
	clientAttributes *func(hub *WebSocketHub, r *http.Request) (*ClientAttributes, error)
	welcomeMessages  *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context) ([][]byte, error)
	onConnect        *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context)
	onDisconnect     *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error, ctx context.Context)
	onError          *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error, ctx context.Context)
}
type funcHandlerOption struct {
	f func(*handlerOptions)
}

func (fdo *funcHandlerOption) apply(do *handlerOptions) {
	fdo.f(do)
}

func newFuncHandlerOption(f func(*handlerOptions)) *funcHandlerOption {
	return &funcHandlerOption{f: f}
}

func WithUhttpHandler(h uhttp.Handler) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.uhttpHandler = h
	})
}

func WithClientAttributes(f func(hub *WebSocketHub, r *http.Request) (*ClientAttributes, error)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.clientAttributes = &f
	})
}

func WithWelcomeMessages(f func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context) ([][]byte, error)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.welcomeMessages = &f
	})
}

func WithOnConnect(f func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onConnect = &f
	})
}

func WithOnDisconnect(f func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onDisconnect = &f
	})
}

func WithOnError(f func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onError = &f
	})
}
