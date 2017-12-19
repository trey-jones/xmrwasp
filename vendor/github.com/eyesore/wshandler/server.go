package wshandler

import "net/http"

// Server responds to Websocket connection events
type Server interface {
	// Conn and SetConn allow us to expose the underlying connection
	// This is needed for wrting especially, and allows us to expose some other methods like Close
	Conn() *Conn
	SetConn(*Conn)

	OnConnect(*http.Request) error

	OnOpen() error
	// OnMessage will receive: payload, isBinary
	OnMessage([]byte, bool) error

	// OnClose will receive: wasClean, code, reason
	OnClose(bool, int, error) error
}

// ServerFactory creates a server
type ServerFactory func() (Server, error)
