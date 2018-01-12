package stratum

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/rpc"
	"strings"
	"sync"

	"github.com/powerman/rpc-codec/jsonrpc2"
)

// DefaultServerCodec handles xmr stratum+tcp requests and is capabable of sending a notification to
// the connection using it.
type DefaultServerCodec struct {
	*serverCodec

	// JSON-RPC clients can use arbitrary json values as request IDs.
	// Package rpc expects uint64 request IDs.
	// We assign uint64 sequence numbers to incoming requests
	// but save the original request ID in the pending map.
	// When rpc responds, we use the sequence number in
	// the response to find the original request ID.
	mutex   sync.Mutex // protects seq, pending
	seq     uint64
	pending map[uint64]*json.RawMessage
}

type defaultNotification struct {
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

// NewDefaultServerCodec returns a new rpc.ServerCodec for handling from a miner implementing the
// (standard?) xmr stratum+tcp protocol
func NewDefaultServerCodec(conn io.ReadWriteCloser) rpc.ServerCodec {
	return &DefaultServerCodec{
		serverCodec: &serverCodec{
			dec: json.NewDecoder(conn),
			enc: json.NewEncoder(conn),
			c:   conn,
			ctx: context.Background(),
		},
		pending: make(map[uint64]*json.RawMessage),
	}
}

// NewDefaultServerCodecContext is NewDefaultServerCodec with given context provided
// within parameters for compatible RPC methods.
func NewDefaultServerCodecContext(ctx context.Context, conn io.ReadWriteCloser) rpc.ServerCodec {
	codec := NewDefaultServerCodec(conn)
	codec.(*DefaultServerCodec).ctx = ctx
	return codec
}

// ReadRequestHeader implements rpc.ServerCodec
func (c *DefaultServerCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	var raw json.RawMessage
	if err := c.dec.Decode(&raw); err != nil {
		c.encmutex.Lock()
		c.enc.Encode(serverResponse{Version: "2.0", ID: &null, Error: errParse})
		c.encmutex.Unlock()
		return err
	}

	if err := json.Unmarshal(raw, &c.req); err != nil {
		if err.Error() == "bad request" {
			c.encmutex.Lock()
			c.enc.Encode(serverResponse{Version: "2.0", ID: &null, Error: errRequest})
			c.encmutex.Unlock()
		}
		return err
	}

	r.ServiceMethod = strings.Title(c.req.Method)
	if !strings.Contains(r.ServiceMethod, "mining") {
		r.ServiceMethod = "mining." + r.ServiceMethod
	}

	// JSON request id can be any JSON value;
	// RPC package expects uint64.  Translate to
	// internal uint64 and save JSON on the side.
	c.mutex.Lock()
	c.seq++
	c.pending[c.seq] = c.req.ID
	c.req.ID = nil
	r.Seq = c.seq
	c.mutex.Unlock()

	return nil
}

// ReadRequestBody implements rpc.ServerCodec
func (c *DefaultServerCodec) ReadRequestBody(x interface{}) error {
	if x == nil {
		return nil
	}
	if x, ok := x.(jsonrpc2.WithContext); ok {
		x.SetContext(c.ctx)
	}
	if c.req.Params == nil {
		return nil
	}
	if err := json.Unmarshal(*c.req.Params, x); err != nil {
		return jsonrpc2.NewError(errParams.Code, err.Error())
	}
	return nil
}

// WriteResponse implements rpc.ServerCodec
func (c *DefaultServerCodec) WriteResponse(r *rpc.Response, x interface{}) error {
	c.mutex.Lock()
	b, ok := c.pending[r.Seq]
	if !ok {
		c.mutex.Unlock()
		return errors.New("invalid sequence number in response")
	}
	delete(c.pending, r.Seq)
	c.mutex.Unlock()

	if b == nil {
		// Notification. Do not respond.
		return nil
	}
	resp := serverResponse{Version: "2.0", ID: b}
	if r.Error == "" {
		if x == nil {
			resp.Result = &null
		} else {
			resp.Result = x
		}
	} else {
		resp.Error = jsonrpc2.NewError(errInternal.Code, r.Error)
	}
	// c.encmutex.Lock()
	// defer c.encmutex.Unlock()
	return c.enc.Encode(resp)
}

// Close implements rpc.ServerCodec
func (c *DefaultServerCodec) Close() error {
	return c.c.Close()
}

func (d *DefaultServerCodec) Notify(method string, args interface{}) error {
	payload := defaultNotification{
		Method: method,
		Params: args,
	}
	return d.enc.Encode(payload)
}
