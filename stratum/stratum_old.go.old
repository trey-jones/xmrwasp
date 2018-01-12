package stratum

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net"

	"go.uber.org/zap"
)

var (
	Delimiter = byte('\n')
)

type Client struct {
	socket net.Conn

	authID   string
	sequence uint64

	// outbound
	Requests chan *Request

	// inbound
	Received chan *Response
	Jobs     chan map[string]interface{}
	Errors   chan error

	logger *zap.SugaredLogger
}

func NewClient(host string, logger *zap.SugaredLogger) (*Client, error) {
	c := &Client{
		Requests: make(chan *Request, 256),
		Received: make(chan *Response, 128),
		Jobs:     make(chan map[string]interface{}, 8),
		Errors:   make(chan error, 8),

		logger: logger,
	}
	socket, err := net.Dial("tcp", host)
	if err != nil {
		return nil, err
	}
	c.socket = socket
	go c.listen()
	go c.connect()

	return c, nil
}

func (c *Client) connect() {
	for r := range c.Requests {
		r.ID = c.nextID()
		if r.Method != "login" {
			r.Params["id"] = c.authID
		}
		rawmsg, err := json.Marshal(r)
		if err != nil {
			c.Errors <- err
			continue
		}
		rawmsg = append(rawmsg, Delimiter)
		c.logger.Debug("Sending: ", string(rawmsg))
		_, err = c.socket.Write(rawmsg)
		if err != nil {
			c.Errors <- err
			continue
		}
	}
}

func (c *Client) listen() {
	defer c.cleanup()
	reader := bufio.NewReader(c.socket)
	for {
		rawmessage, err := reader.ReadBytes(Delimiter)
		if err != nil {
			c.Errors <- err
			return
		}
		c.logger.Debug("Receiving: ", string(rawmessage))
		c.handleMessage(rawmessage)
	}
}

func (c *Client) handleMessage(raw []byte) {
	r := &Response{}
	err := json.Unmarshal(raw, &r)
	if err != nil {
		c.Errors <- err
		return
	}
	if r.Error != nil {
		c.Errors <- r.Error
		return
	}
	if r.ID != 0 {
		if c.authID == "" {
			if authID, ok := r.Result["id"]; ok {
				c.authID = authID.(string)
			}
		}
		if job, ok := r.Result["job"]; ok {
			c.Jobs <- job.(map[string]interface{})
			return
		}
		c.Received <- r
		return
	}
	// comment added later - I'm assuming we use request here because the response has the same format...
	// could rename to Message
	req := Request{}
	err = json.Unmarshal(raw, &req)
	if err != nil {
		c.Errors <- err
		return
	}
	if req.Method == "job" {
		c.Jobs <- req.Params
		return
	}
}

func (c *Client) cleanup() {
	// close connections etc
	fmt.Println("cleaning up")
}

func (c *Client) nextID() uint64 {
	// only call from connect thread - never concurrent
	if float64(c.sequence) > math.Pow(2, 50) {
		// start at 2 - does this break the protocol?
		c.sequence = 1
	}
	c.sequence++
	return c.sequence
}
