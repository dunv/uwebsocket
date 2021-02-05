package uwebsocket

import (
	"net/http"

	"github.com/dunv/uhttp"
)

type Handler struct {
	UhttpHandler     uhttp.Handler
	ClientAttributes *func(hub *WebSocketHub, r *http.Request) (*ClientAttributes, error)
	WelcomeMessages  *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request) ([][]byte, error)
	OnConnect        *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request)
	OnDisconnect     *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error)
	OnError          *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error)
}

func ClientAttributesFunc(clientAttributesFunc func(hub *WebSocketHub, r *http.Request) (*ClientAttributes, error)) *func(hub *WebSocketHub, r *http.Request) (*ClientAttributes, error) {
	return &clientAttributesFunc
}

func WelcomeMessages(welcomeMessagesFunc func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request) ([][]byte, error)) *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request) ([][]byte, error) {
	return &welcomeMessagesFunc
}

func OnConnect(onConnectFunc func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request)) *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request) {
	return &onConnectFunc
}

func OnDisconnect(onDisconnectFunc func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error)) *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error) {
	return &onDisconnectFunc
}

func OnError(onErrorFunc func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error)) *func(hub *WebSocketHub, clientGuid string, clientAttributes *ClientAttributes, r *http.Request, err error) {
	return &onErrorFunc
}
