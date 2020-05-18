package uwebsocket

type ClientAttributes map[string]interface{}

type ClientMessage struct {
	Message []byte
	Client  ClientAttributes
}
