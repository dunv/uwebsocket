package uwebsocket

import (
	"net/http"

	"github.com/dunv/uhttp"
)

type Handler struct {
	UhttpHandler     uhttp.Handler
	ClientAttributes *func(hub *WebSocketHub, r *http.Request) (ClientAttributes, error)
	OnConnect        *func(hub *WebSocketHub, clientAttributes ClientAttributes, r *http.Request)
	OnDisconnect     *func(hub *WebSocketHub, clientAttributes ClientAttributes, err error)
	OnError          *func(err error)
}

func ClientAttributesFunc(clientAttributesFunc func(hub *WebSocketHub, r *http.Request) (ClientAttributes, error)) *func(hub *WebSocketHub, r *http.Request) (ClientAttributes, error) {
	return &clientAttributesFunc
}

func OnConnect(onConnectFunc func(hub *WebSocketHub, clientAttributes ClientAttributes, r *http.Request)) *func(hub *WebSocketHub, clientAttributes ClientAttributes, r *http.Request) {
	return &onConnectFunc
}

func OnDisconnect(onDisconnectFunc func(hub *WebSocketHub, clientAttributes ClientAttributes, err error)) *func(hub *WebSocketHub, clientAttributes ClientAttributes, err error) {
	return &onDisconnectFunc
}

func OnError(onErrorFunc func(err error)) *func(err error) {
	return &onErrorFunc
}
