package stratum

import (
	"context"
	"encoding/json"
	"io"
	"net/rpc"
	"strings"

	"github.com/powerman/rpc-codec/jsonrpc2"
)

type CoinhiveServerCodec struct {
	*serverCodec

	req chServerRequest
}

// NewCoinhiveServerCodec returns a new rpc.ServerCodec for handling requests from the Coinhive Miner
func NewCoinhiveServerCodec(conn io.ReadWriteCloser) rpc.ServerCodec {
	return &CoinhiveServerCodec{
		serverCodec: &serverCodec{
			dec: json.NewDecoder(conn),
			enc: json.NewEncoder(conn),
			c:   conn,
			ctx: context.Background(),
		},
	}
}

// NewCoinhiveServerCodecContext is NewCoinhiveServerCodec with given context provided
// within parameters for compatible RPC methods.
func NewCoinhiveServerCodecContext(ctx context.Context, conn io.ReadWriteCloser) rpc.ServerCodec {
	codec := NewCoinhiveServerCodec(conn)
	codec.(*CoinhiveServerCodec).ctx = ctx
	return codec
}

type chServerRequest struct {
	Version string           `json:"jsonrpc"`
	Method  string           `json:"type"`
	Params  *json.RawMessage `json:"params"`
	ID      *json.RawMessage `json:"id"`
}

type chServerResponse struct {
	Method string      `json:"type"`
	Result interface{} `json:"params,omitempty"`
	Error  interface{} `json:"error,omitempty"`
}

type chNotification struct {
	Method string      `json:"type"`
	Params interface{} `json:"params"`
}

// ReadRequestHeader implements rpc.ServerCodec
func (c *CoinhiveServerCodec) ReadRequestHeader(r *rpc.Request) (err error) {
	// var raw json.RawMessage
	if err := c.dec.Decode(&c.req); err != nil {
		c.encmutex.Lock()
		c.enc.Encode(chServerResponse{Error: errParse})
		c.encmutex.Unlock()
		return err
	}

	// if err := json.Unmarshal(raw, &c.req); err != nil {
	// 	if err.Error() == "bad request" {
	// 		c.encmutex.Lock()
	// 		c.enc.Encode(chServerResponse{Error: errRequest})
	// 		c.encmutex.Unlock()
	// 	}
	// 	return err
	// }

	r.ServiceMethod = strings.Title(c.req.Method)
	if !strings.Contains(r.ServiceMethod, "mining") {
		r.ServiceMethod = "mining." + r.ServiceMethod
	}

	return nil
}

// ReadRequestBody implements rpc.ServerCodec
func (c *CoinhiveServerCodec) ReadRequestBody(x interface{}) error {
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
func (c *CoinhiveServerCodec) WriteResponse(r *rpc.Response, x interface{}) error {
	resp := chServerResponse{Method: c.req.Method}
	if r.Error == "" {
		if x == nil {
			resp.Result = &null
		} else {
			resp.Result = x
		}
	} else {
		raw := json.RawMessage(r.Error)
		resp.Error = &raw
	}

	// still not sure mutex is necessary
	// c.encmutex.Lock()
	// defer c.encmutex.Unlock()
	return c.enc.Encode(resp)
}

// Close implements rpc.ServerCodec
func (c *CoinhiveServerCodec) Close() error {
	return c.c.Close()
}

func (c *CoinhiveServerCodec) Notify(method string, args interface{}) error {
	payload := chNotification{
		Method: method,
		Params: args,
	}
	// c.encmutex.Lock()
	// defer c.encmutex.Unlock()
	return c.enc.Encode(payload)
}
