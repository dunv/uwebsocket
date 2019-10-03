package uwebsocket

import (
	"net/http"

	"github.com/dunv/uhttp"
)

type Handler struct {
	UhttpHandler     uhttp.Handler
	ClientAttributes ClientAttributes
	OnConnect        *func(r *http.Request)
}

func OnConnect(onConnectFunc func(r *http.Request)) *func(r *http.Request) {
	return &onConnectFunc
}
