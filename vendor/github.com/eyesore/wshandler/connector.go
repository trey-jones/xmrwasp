package wshandler

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var (
	// debug can be set to true to enable verbose http errors
	// TODO add debug logger
	debug = false
)

// SetDebug turns debug output on or off
func SetDebug(on bool) {
	debug = on
}

func onConnect(h Handler, r *http.Request) error {
	return h.OnConnect(r)
}

func onOpen(h Handler) error {
	c := h.Conn()
	c.Conn.SetReadLimit(c.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(c.PongTimeout))

	// PongHandler may be modified by h.OnOpen - this is the default
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(c.PongTimeout))
		return nil
	})

	err := h.OnOpen()
	if err != nil {
		return err
	}
	go startReading(h)
	go startWriting(h)
	return nil
}

func onMessage(s Server, payload []byte, isBinary bool) error {
	return s.OnMessage(payload, isBinary)
}

func onClose(h Handler, wasClean bool, code int, reason error) error {
	return h.OnClose(wasClean, code, reason)
}

func startReading(h Handler) {
	c := h.Conn()
	defer close(c.inbox)
	// var reason error
	// code := websocket.CloseNormalClosure
	// wasClean := true
	s, isServer := h.(Server)
	for {
		mtype, m, err := c.Conn.ReadMessage()
		if err != nil {
			// wasClean = false
			// reason = err
			// code = websocket.CloseGoingAway
			break
		}
		isBinary := mtype == websocket.BinaryMessage
		if !isServer {
			c.inbox <- &message{m, isBinary, nil}
		} else {
			err = onMessage(s, m, isBinary)
			if err != nil {
				// wasClean = false
				// reason = err
				// code = websocket.CloseInternalServerErr
				break
			}
		}
	}
	// // onClose(s, wasClean, code, reason)
	// c.Conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(c.PingInterval))
	// c.Conn.Close()
	timeout := time.NewTimer(30 * time.Second)
	select {
	case c.closeSignal <- true:
	case <-timeout.C:
	}
}

func startWriting(h Handler) {
	c := h.Conn()
	// TODO learn about close codes
	var reason error
	code := websocket.CloseNormalClosure
	wasClean := true
	ticker := time.NewTicker(c.PingInterval)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
		// avoids race conditions with onclose - not ideal!
		onClose(h, wasClean, code, reason)
	}()
	for {
		select {
		case out := <-c.outbox:
			var messageType int
			if out.isBinary {
				messageType = websocket.BinaryMessage
			} else {
				messageType = websocket.TextMessage
			}
			c.Conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))
			w, err := c.Conn.NextWriter(messageType)
			if err != nil {
				out.resp <- &writeResponse{0, err}
				wasClean = false
				reason = err
				// don't know if this is the right code
				code = websocket.CloseGoingAway
				return
			}
			written, err := w.Write(out.content)
			if err != nil {
				out.resp <- &writeResponse{written, err}
				wasClean = false
				reason = err
				// don't know if this is the right code
				code = websocket.CloseGoingAway
				return
			}
			select {
			case out.resp <- &writeResponse{written, w.Close()}:
			case <-time.After(c.WriteTimeout):
			}
		case <-ticker.C:
			err := c.Conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.WriteTimeout))
			if err != nil {
				wasClean = false
				reason = err
				code = websocket.CloseGoingAway
				return
			}
		case <-c.closeSignal:
			// TODO allow configurable data in control messages?
			err := c.Conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(c.WriteTimeout))
			if err != nil {
				wasClean = true
				reason = err
				code = websocket.CloseGoingAway
			}
			return
		}
	}
}

// Connector is an http.Handler that creates and spits out new WS connections
type Connector struct {
	// Set options on Upgrader to configure
	Upgrader *websocket.Upgrader
	f        Factory

	// default options will be passed on to each Server yielded by this Connector
	// PingInterval is how often we send a ping frame to make sure someone is still listening
	PingInterval time.Duration

	// PongTimeout is how long after sending a ping frame we will wait for a pong frame before closing the connection
	PongTimeout time.Duration

	MaxMessageSize int64
	WriteTimeout   time.Duration

	// ReadBufferSize determines the buffer size of the inbox channel
	// The purpose of the read buffer is to detect instances that are not consuming the
	// read buffer if used. Increase this if the buffer is filling faster than you can
	// consume it.
	ReadBufferSize int
}

// NewConnector returns a usable and configurable Connector
func NewConnector(f Factory) *Connector {
	ctr := &Connector{
		Upgrader: &websocket.Upgrader{
			HandshakeTimeout: 30 * time.Second,
		},
		f: f,

		// default options
		PingInterval:   30 * time.Second,
		PongTimeout:    60 * time.Second,
		MaxMessageSize: 4096,
		WriteTimeout:   15 * time.Second,
		ReadBufferSize: 100,
	}

	return ctr
}

// AllowAnyOrigin causes the Conn not to reject any connection attempts based on origin
func (ctr *Connector) AllowAnyOrigin() {
	ctr.Upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}
}

// ServeHTTP satisfies http.Handler - errors just write 500 InternalServerError
func (ctr *Connector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	onError := func(e error) {
		errorText := e.Error()
		if !debug {
			errorText = http.StatusText(http.StatusInternalServerError)
		}
		http.Error(w, errorText, http.StatusInternalServerError)
	}
	h, err := ctr.f()
	c := &Conn{
		PingInterval:   ctr.PingInterval,
		PongTimeout:    ctr.PongTimeout,
		MaxMessageSize: ctr.MaxMessageSize,
		WriteTimeout:   ctr.WriteTimeout,

		ResponseHeader: http.Header{},

		outbox:      make(chan *message),
		inbox:       make(chan *message, ctr.ReadBufferSize),
		closeSignal: make(chan bool),
	}
	h.SetConn(c)
	err = onConnect(h, r)
	if err != nil {
		onError(err)
		return
	}

	wsconn, err := ctr.Upgrader.Upgrade(w, r, c.ResponseHeader)
	if err != nil {
		// http error is written by Upgrader
		return
	}
	c.Conn = wsconn

	onOpen(h)
}
