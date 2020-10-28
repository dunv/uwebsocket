package uwebsocket

type ClientMessage struct {
	Message          []byte
	ClientGUID       string
	ClientAttributes *ClientAttributes
}
