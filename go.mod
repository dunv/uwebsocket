module github.com/dunv/uwebsocket

go 1.13

require (
	github.com/dunv/uauth v1.0.38
	github.com/dunv/uhelpers v1.0.4
	github.com/dunv/uhttp v1.0.29
	github.com/dunv/ulog v0.0.13
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/google/uuid v1.1.1
	github.com/gorilla/websocket v1.4.1
	github.com/stretchr/testify v1.4.0 // indirect
	go.mongodb.org/mongo-driver v1.1.1
)

replace github.com/dunv/uhttp => ../uhttp
