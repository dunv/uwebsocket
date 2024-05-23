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
	uhttpHandler      uhttp.Handler
	clientAttributes  *func(hub *webSocketHub, r *http.Request) (*ClientAttributes, error)
	welcomeMessages   *func(hub *webSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context) ([][]byte, error)
	onConnect         *func(hub *webSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context)
	onError           *func(hub *webSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error, ctx context.Context)
	onIncomingMessage *func(hub *webSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, msg ClientMessage, ctx context.Context)
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

func WithClientAttributes(f func(hub *webSocketHub, r *http.Request) (*ClientAttributes, error)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.clientAttributes = &f
	})
}

func WithWelcomeMessages(f func(hub *webSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context) ([][]byte, error)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.welcomeMessages = &f
	})
}

func WithOnConnect(f func(hub *webSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onConnect = &f
	})
}

func WithOnError(f func(hub *webSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onError = &f
	})
}

func WithOnIncomingMessage(f func(hub *webSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, msg ClientMessage, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onIncomingMessage = &f
	})
}
