package stratum

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/rpc"
	"sync"
)

type serverCodec struct {
	encmutex sync.Mutex    // protects enc
	dec      *json.Decoder // for reading JSON values
	enc      *json.Encoder // for writing JSON values
	c        io.ReadWriteCloser
	ctx      context.Context

	// temporary work space
	req serverRequest
}

type serverRequest struct {
	Version string           `json:"jsonrpc"`
	Method  string           `json:"method"`
	Params  *json.RawMessage `json:"params"`
	ID      *json.RawMessage `json:"id"`
}

func (r *serverRequest) reset() {
	r.Version = ""
	r.Method = ""
	r.Params = nil
	r.ID = nil
}

func (r *serverRequest) UnmarshalJSON(raw []byte) error {
	r.reset()
	type req *serverRequest
	if err := json.Unmarshal(raw, req(r)); err != nil {
		return errors.New("bad request")
	}

	var o = make(map[string]*json.RawMessage)
	if err := json.Unmarshal(raw, &o); err != nil {
		return errors.New("bad request")
	}
	// if o["type"] == nil {

	// 	return errors.New("bad request")
	// }
	_, okID := o["id"]
	_, okParams := o["params"]
	// if len(o) == 3 && !(okID || okParams) || len(o) == 4 && !(okID && okParams) || len(o) > 4 {
	// 	return errors.New("bad request")
	// }
	if okParams {
		if r.Params == nil || len(*r.Params) == 0 {
			return errors.New("bad request")
		}
		switch []byte(*r.Params)[0] {
		case '[', '{':
		default:
			return errors.New("bad request")
		}
	}
	if okID && r.ID == nil {
		r.ID = &null
	}
	if okID {
		if len(*r.ID) == 0 {
			return errors.New("bad request")
		}
		switch []byte(*r.ID)[0] {
		case 't', 'f', '{', '[':
			return errors.New("bad request")
		}
	}

	return nil
}

type serverResponse struct {
	Version string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  interface{}      `json:"result,omitempty"`
	Error   interface{}      `json:"error,omitempty"`
}

// public API

type Server struct {
	*rpc.Server
}

func NewServer() *Server {
	s := &Server{
		rpc.NewServer(),
	}
	return s
}

func (s *Server) ServeCodec(codec rpc.ServerCodec) {
	// defer codec.Close()
	s.Server.ServeCodec(codec)
}

func (s *Server) ServeConn(ctx context.Context, conn io.ReadWriteCloser) {
	s.ServeCodec(NewDefaultServerCodecContext(ctx, conn))
}
