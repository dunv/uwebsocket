package uwebsocket

import (
	"net/http"

	"github.com/dunv/uhttp"
)

type Handler struct {
	UhttpHandler     uhttp.Handler
	ClientAttributes *func(hub *WebSocketHub, r *http.Request) (ClientAttributes, error)
	OnConnect        *func(hub *WebSocketHub, clientAttributes ClientAttributes, r *http.Request)
}

func ClientAttributesFunc(clientAttributesFunc func(hub *WebSocketHub, r *http.Request) (ClientAttributes, error)) *func(hub *WebSocketHub, r *http.Request) (ClientAttributes, error) {
	return &clientAttributesFunc
}

func OnConnect(onConnectFunc func(hub *WebSocketHub, clientAttributes ClientAttributes, r *http.Request)) *func(hub *WebSocketHub, clientAttributes ClientAttributes, r *http.Request) {
	return &onConnectFunc
}
