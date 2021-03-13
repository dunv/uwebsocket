package uwebsocket

import (
	"github.com/dunv/ulog"
)

func init() {
	ulog.AddReplaceFunction("github.com/dunv/uwebsocket.(*WebSocketHub).Handle", "uwebsocket.Handle")
	ulog.AddReplaceFunction("github.com/dunv/uwebsocket.(*WebSocketClient).writePump", "uwebsocket.ClientWrite")
	ulog.AddReplaceFunction("github.com/dunv/uwebsocket.(*WebSocketClient).readPump", "uwebsocket.ClientRead")
}
