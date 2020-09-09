package uwebsocket

import (
	"fmt"
	"net/http"
	"strings"
)

func UpgradeConnection(
	hub *WebSocketHub,
	handler *Handler,
	clientGuid string,
	clientAttributes ClientAttributes,
	w http.ResponseWriter,
	r *http.Request,
) error {
	upgrader.CheckOrigin = func(r *http.Request) bool {
		if config.CORS == nil {
			return true
		}
		if *config.CORS == "*" {
			return true
		}

		for _, host := range strings.Split(*config.CORS, ",") {
			if host == r.Host {
				return true
			}
		}

		return false
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("Could not Upgrade connection (%s)", err)
	}
	client := &WebSocketClient{
		hub:            hub,
		conn:           conn,
		send:           make(chan []byte, 256),
		clientGuid:     clientGuid,
		attributes:     clientAttributes,
		connectRequest: r,
		handler:        handler,
	}
	client.hub.register <- client

	go client.writePump()
	go client.readPump()
	return nil
}
