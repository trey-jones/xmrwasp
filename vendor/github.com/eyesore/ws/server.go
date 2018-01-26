package ws

import "net/http"

// Connector handles Websocket connections by calling back on Connect, Open, and Close
type Connector interface {
	// Conn and SetConn expose the underlying connection
	Conn() *Conn
	SetConn(*Conn)

	// callbacks
	OnConnect(*http.Request) error
	OnOpen() error
	OnClose(wasClean bool, code int, reason error) error
}

// Server serves Websocket connections by calling back on Connect, Open, Message, and Close
type Server interface {
	Connector

	// OnMessage is called each time a message is received on the Conn if implemented.
	// If implemented, Read should not be called as it will not receive the messages
	// and will instead block indefinitely.
	// TODO allow configuration to use both?
	OnMessage(payload []byte, isBinary bool) error
}

// Factory creates a server
type Factory func() (Connector, error)
