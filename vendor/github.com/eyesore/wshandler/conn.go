package wshandler

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Conn exposes per-socket connection configs, and the Write and Close methods
type Conn struct {
	Conn *websocket.Conn
	// ResponseHeader can be modified eg. in OnConnect to be included in the initial http response
	ResponseHeader http.Header

	// PingInterval is how often we send a ping frame to make sure someone is still listening
	PingInterval time.Duration

	// PongTimeout is how long after sending a ping frame we will wait for a pong frame before closing the connection
	PongTimeout time.Duration

	MaxMessageSize int64
	WriteTimeout   time.Duration

	outbox      chan message
	closeSignal chan bool
}

func (c *Conn) Write(m []byte, isBinary bool) {
	// TODO log type
	lo.Debug("Sending message: ", string(m))
	out := message{m, isBinary}
	// give the thread some time to respond, but kill it if it's obviously dead
	timeout := time.NewTimer(c.WriteTimeout)
	select {
	case c.outbox <- out:
	case <-timeout.C:
	}
}

// Close causes the connection to close.  How about that?
func (c *Conn) Close() {
	timeout := time.NewTimer(30 * time.Second)
	select {
	case c.closeSignal <- true:
	case <-timeout.C:
	}
}

// Message contains the binary message to be sent and whether it should be interpreted as binary rather than text
type message struct {
	content  []byte
	isBinary bool
}
