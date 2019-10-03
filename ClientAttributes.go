package uwebsocket

type ClientAttributes interface{}

type ClientMessage struct {
	Message []byte
	Client  ClientAttributes
}
