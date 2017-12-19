package wshandler

import (
	"log"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/gorilla/websocket"
)

var (
	// debug can be set to true to enable logging and verbose http errors
	debug = false
	lo    *zap.SugaredLogger
)

func init() {
	initlogger()
}

func initlogger() {
	var l *zap.Logger
	var err error
	if debug {
		l, err = zap.NewDevelopment()
	} else {
		l, err = zap.NewProduction()
	}
	if err != nil {
		log.Fatal("Unable to initialize lo (for some reason)")
	}
	lo = l.Sugar()
}

// SetDebug turns debug output on or off
func SetDebug(on bool) {
	debug = on
	initlogger()
}

func onConnect(s Server, r *http.Request) error {
	lo.Debug("Attempting socket connection.")
	return s.OnConnect(r)
}

func onOpen(s Server) error {
	lo.Debug("Opened new connection.")
	c := s.Conn()
	c.Conn.SetReadLimit(c.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(c.PongTimeout))

	// PongHandler may be modified by s.OnOpen - this is the default
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(c.PongTimeout))
		return nil
	})

	lo.Debug("Starting socket i/o")
	err := s.OnOpen()
	if err != nil {
		return err
	}
	go startReading(s)
	go startWriting(s)
	return nil
}

func onMessage(s Server, payload []byte, isBinary bool) error {
	lo.Debug("Received message: ", string(payload))
	return s.OnMessage(payload, isBinary)
}

func onClose(s Server, wasClean bool, code int, reason error) error {
	lo.Debug("Closing channel.  Clean? ", wasClean, " --- Reason: ", reason)
	return s.OnClose(wasClean, code, reason)
}

func startReading(s Server) {
	c := s.Conn()
	// var reason error
	// code := websocket.CloseNormalClosure
	// wasClean := true
	for {
		mtype, m, err := c.Conn.ReadMessage()
		if err != nil {
			// wasClean = false
			// reason = err
			// code = websocket.CloseGoingAway
			break
		}
		err = onMessage(s, m, mtype == websocket.BinaryMessage)
		if err != nil {
			// wasClean = false
			// reason = err
			// code = websocket.CloseInternalServerErr
			break
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

func startWriting(s Server) {
	c := s.Conn()
	// TODO learn about close codes
	var reason error
	code := websocket.CloseNormalClosure
	wasClean := true
	ticker := time.NewTicker(c.PingInterval)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
		// avoids race conditions with onclose - not ideal!
		onClose(s, wasClean, code, reason)
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
			err := c.Conn.WriteMessage(messageType, out.content)
			if err != nil {
				wasClean = false
				reason = err
				// don't know if this is the right code
				code = websocket.CloseGoingAway
				return
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

// Connector is an http.Handler that creates and spits out new WS connections
type Connector struct {
	// Set options on Upgrader to configure
	Upgrader *websocket.Upgrader
	f        ServerFactory

	// default options will be passed on to each Server yielded by this Connector
	// PingInterval is how often we send a ping frame to make sure someone is still listening
	PingInterval time.Duration

	// PongTimeout is how long after sending a ping frame we will wait for a pong frame before closing the connection
	PongTimeout time.Duration

	MaxMessageSize int64
	WriteTimeout   time.Duration
}

// NewConnector returns a usable and configurable Connector
func NewConnector(f ServerFactory) *Connector {
	ctr := &Connector{
		Upgrader: &websocket.Upgrader{
			HandshakeTimeout: 30 * time.Second,
		},
		f: f,

		// default options
		PingInterval:   30 * time.Second,
		PongTimeout:    60 * time.Second,
		MaxMessageSize: 2048,
		WriteTimeout:   15 * time.Second,
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
	s, err := ctr.f()
	c := &Conn{
		PingInterval:   ctr.PingInterval,
		PongTimeout:    ctr.PongTimeout,
		MaxMessageSize: ctr.MaxMessageSize,
		WriteTimeout:   ctr.WriteTimeout,

		ResponseHeader: http.Header{},

		outbox:      make(chan message),
		closeSignal: make(chan bool),
	}
	s.SetConn(c)
	err = onConnect(s, r)
	if err != nil {
		onError(err)
		return
	}

	wsconn, err := ctr.Upgrader.Upgrade(w, r, c.ResponseHeader)
	if err != nil {
		// error is written by Upgrader
		return
	}
	c.Conn = wsconn

	onOpen(s)
}
