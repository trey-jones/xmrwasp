package wshandler

import "net/http"

// Handler handles Websocket connections by calling back on Connect, Open, and Close
type Handler interface {
	// Conn and SetConn allow us to expose the underlying connection
	// This is needed for writing especially, and allows us to expose some other methods like Close
	Conn() *Conn
	SetConn(*Conn)
	OnConnect(*http.Request) error
	OnOpen() error
	OnClose(wasClean bool, code int, reason error) error
}

// Server serves Websocket connections by calling back on Connect, Open, Message, and Close
type Server interface {
	Handler
	OnMessage(payload []byte, isBinary bool) error
}

// Factory creates a server
type Factory func() (Handler, error)
