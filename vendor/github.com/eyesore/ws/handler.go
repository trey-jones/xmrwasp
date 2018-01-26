package ws

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

func onConnect(ctr Connector, r *http.Request) error {
	return ctr.OnConnect(r)
}

func onOpen(ctr Connector) error {
	c := ctr.Conn()
	c.Conn.SetReadLimit(c.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(c.PongTimeout))

	// PongHandler may be modified by ctr.OnOpen - this is the default
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(c.PongTimeout))
		return nil
	})

	err := ctr.OnOpen()
	if err != nil {
		return err
	}
	go startReading(ctr)
	go startWriting(ctr)
	return nil
}

func onMessage(s Server, payload []byte, isBinary bool) error {
	return s.OnMessage(payload, isBinary)
}

func onClose(ctr Connector, wasClean bool, code int, reason error) error {
	return ctr.OnClose(wasClean, code, reason)
}

func startReading(ctr Connector) {
	c := ctr.Conn()
	defer close(c.inbox)
	// var reason error
	// code := websocket.CloseNormalClosure
	// wasClean := true
	s, isServer := ctr.(Server)
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

func startWriting(ctr Connector) {
	c := ctr.Conn()
	// TODO learn about close codes
	var reason error
	code := websocket.CloseNormalClosure
	wasClean := true
	ticker := time.NewTicker(c.PingInterval)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
		// avoids race conditions with onclose - not ideal!
		onClose(ctr, wasClean, code, reason)
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
			err := c.Conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(c.WriteTimeout))
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

// Handler is an http.Handler that creates and spits out new WS connections
type Handler struct {
	// Set options on Upgrader to configure
	Upgrader *websocket.Upgrader
	f        Factory

	// default options will be passed on to each Server yielded by this Handler
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

// NewHandler returns a usable and configurable Handler
func NewHandler(f Factory) *Handler {
	h := &Handler{
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

	return h
}

// AllowAnyOrigin causes the Conn not to reject any connection attempts based on origin
func (h *Handler) AllowAnyOrigin() {
	h.Upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}
}

// ServeHTTP satisfies http.Handler - errors just write 500 InternalServerError
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	onError := func(e error) {
		errorText := e.Error()
		if !debug {
			errorText = http.StatusText(http.StatusInternalServerError)
		}
		http.Error(w, errorText, http.StatusInternalServerError)
	}
	ctr, err := h.f()
	c := &Conn{
		PingInterval:   h.PingInterval,
		PongTimeout:    h.PongTimeout,
		MaxMessageSize: h.MaxMessageSize,
		WriteTimeout:   h.WriteTimeout,

		ResponseHeader: http.Header{},

		outbox:      make(chan *message),
		inbox:       make(chan *message, h.ReadBufferSize),
		closeSignal: make(chan bool),
	}
	ctr.SetConn(c)
	err = onConnect(ctr, r)
	if err != nil {
		onError(err)
		return
	}

	wsconn, err := h.Upgrader.Upgrade(w, r, c.ResponseHeader)
	if err != nil {
		// http error is written by Upgrader
		return
	}
	c.Conn = wsconn

	onOpen(ctr)
}
