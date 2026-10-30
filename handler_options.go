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
	uhttpHandler         uhttp.Handler
	clientAttributes     *func(hub WebSocketHub, r *http.Request) (*ClientAttributes, error)
	welcomeMessages      *func(hub WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context) ([][]byte, error)
	initialSnapshot      *initialSnapshotOptions
	onConnect            *func(hub WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context)
	onError              *func(hub WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error, ctx context.Context)
	onIncomingMessage    *func(hub WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, msg ClientMessage, ctx context.Context)
	incomingMessageLimit int
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

func WithClientAttributes(f func(hub WebSocketHub, r *http.Request) (*ClientAttributes, error)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.clientAttributes = &f
	})
}

// WithWelcomeMessages builds a best-effort welcome asynchronously for each
// client. Live messages may arrive first; use WithInitialSnapshot for ordering.
func WithWelcomeMessages(f func(hub WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context) ([][]byte, error)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.welcomeMessages = &f
	})
}

func WithOnConnect(f func(hub WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onConnect = &f
	})
}

func WithOnError(f func(hub WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onError = &f
	})
}

// WithIncomingMessageQueue sets the per-client waiting callback limit (default
// 64). Frames are already limited to 512 bytes. Overflow disconnects only that
// client. Callbacks must honor their context; cancellation cannot undo work.
func WithIncomingMessageQueue(messages int) HandlerOption {
	if messages <= 0 {
		panic("uwebsocket: incoming message queue must be positive")
	}
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.incomingMessageLimit = messages
	})
}

// WithOnIncomingMessage runs callbacks sequentially per client, independently of
// other clients and the hub event loop. Callbacks across clients may overlap.
func WithOnIncomingMessage(f func(hub WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, msg ClientMessage, ctx context.Context)) HandlerOption {
	return newFuncHandlerOption(func(o *handlerOptions) {
		o.onIncomingMessage = &f
	})
}
