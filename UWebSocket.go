package uwebsocket

import (
	"github.com/dunv/uhttp/logging"
	"github.com/dunv/ulog"
)

var config Config = Config{
	CORS:      nil,
	CustomLog: ulog.NewUlog(),
}

type Config struct {
	CORS      *string
	CustomLog ulog.ULogger
}

func GetConfig() Config {
	return config
}

// SetConfig set config for all handlers
func SetConfig(_config Config) {
	config = _config

	if _config.CustomLog != nil {
		config.CustomLog = _config.CustomLog
		logging.Logger = _config.CustomLog
	}

	if _config.CORS != nil {
		config.CORS = _config.CORS
	}
}

func init() {
	ulog.AddReplaceFunction("github.com/dunv/uwebsocket.(*WebSocketHub).Handle", "uwebsocket.Handle")
	ulog.AddReplaceFunction("github.com/dunv/uwebsocket.(*WebSocketClient).writePump", "uwebsocket.ClientWrite")
	ulog.AddReplaceFunction("github.com/dunv/uwebsocket.(*WebSocketClient).readPump", "uwebsocket.ClientRead")
}
