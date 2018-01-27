package stratum

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"net/rpc"
	"os"
	"reflect"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/powerman/rpc-codec/jsonrpc2"
)

const (
	seqNotify                = math.MaxUint64
	notificationBufferLength = 10
)

var (
	null = json.RawMessage([]byte("null"))

	// CallTimeout is the amount of time we wait for a response before we return an error
	CallTimeout = 30 * time.Second

	// ErrCallTimedOut means that call did not succeed within CallTimeout
	ErrCallTimedOut = errors.New("rpc call timeout")
)

type clientCodec struct {
	dec *json.Decoder // for reading JSON values
	enc *json.Encoder // for writing JSON values
	c   io.ReadWriteCloser

	// temporary work space
	resp  clientResponse
	notif Notification

	// JSON-RPC responses include the request id but not the request method.
	// Package rpc expects both.
	// We save the request method in pending when sending a request
	// and then look it up by request ID when filling out the rpc Response.
	mutex   sync.Mutex        // protects pending
	pending map[uint64]string // map request id to method name

	notifications chan Notification
}

// newClientCodec returns a new rpc.ClientCodec using JSON-RPC 2.0 on conn.
func newClientCodec(conn io.ReadWriteCloser) rpc.ClientCodec {
	return &clientCodec{
		dec:     json.NewDecoder(conn),
		enc:     json.NewEncoder(conn),
		c:       conn,
		pending: make(map[uint64]string),

		// if the buffer gets full, we assume that it's not being consumed and error out
		notifications: make(chan Notification, notificationBufferLength),
	}
}

type clientRequest struct {
	Version string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      *uint64     `json:"id,omitempty"`
}

type Notification clientRequest

func (c *clientCodec) WriteRequest(r *rpc.Request, param interface{}) error {
	if r.Seq == 0 {
		// seems many stratum pools don't like seq = 0
		return errors.New("skipping first request")
	}
	// If return error: it will be returned as is for this call.
	// Allow param to be only Array, Slice, Map or Struct.
	// When param is nil or uninitialized Map or Slice - omit "params".
	if param != nil {
		switch k := reflect.TypeOf(param).Kind(); k {
		case reflect.Map:
			if reflect.TypeOf(param).Key().Kind() == reflect.String {
				if reflect.ValueOf(param).IsNil() {
					param = nil
				}
			}
		case reflect.Slice:
			if reflect.ValueOf(param).IsNil() {
				param = nil
			}
		case reflect.Array, reflect.Struct:
		case reflect.Ptr:
			switch k := reflect.TypeOf(param).Elem().Kind(); k {
			case reflect.Map:
				if reflect.TypeOf(param).Elem().Key().Kind() == reflect.String {
					if reflect.ValueOf(param).Elem().IsNil() {
						param = nil
					}
				}
			case reflect.Slice:
				if reflect.ValueOf(param).Elem().IsNil() {
					param = nil
				}
			case reflect.Array, reflect.Struct:
			default:
				return jsonrpc2.NewError(errInternal.Code, "unsupported param type: Ptr to "+k.String())
			}
		default:
			return jsonrpc2.NewError(errInternal.Code, "unsupported param type: "+k.String())
		}
	}

	var req clientRequest
	if r.Seq != seqNotify {
		c.mutex.Lock()
		c.pending[r.Seq] = r.ServiceMethod
		c.mutex.Unlock()
		req.ID = &r.Seq
	}
	req.Version = "2.0"
	req.Method = r.ServiceMethod
	req.Params = param
	if err := c.enc.Encode(&req); err != nil {
		return jsonrpc2.NewError(errInternal.Code, err.Error())
	}

	return nil
}

type clientResponse struct {
	Version string           `json:"jsonrpc"`
	ID      *uint64          `json:"id"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpc2.Error  `json:"error,omitempty"`
}

func (r *clientResponse) reset() {
	r.Version = ""
	r.ID = nil
	r.Result = nil
	r.Error = nil
}

func (r *clientResponse) UnmarshalJSON(raw []byte) error {
	r.reset()
	type resp *clientResponse
	if err := json.Unmarshal(raw, resp(r)); err != nil {
		return errors.New("bad response: " + string(raw))
	}

	var o = make(map[string]*json.RawMessage)
	if err := json.Unmarshal(raw, &o); err != nil {
		return errors.New("bad response: " + string(raw))
	}
	_, okVer := o["jsonrpc"]
	_, okID := o["id"]
	_, okRes := o["result"]
	_, okErr := o["error"]
	// this has been updated to allow error and result as part of the response
	if !okVer || !okID || !(okRes || okErr) || len(o) > 4 {
		return errors.New("bad response: " + string(raw))
	}
	if r.Version != "2.0" {
		return errors.New("bad response: " + string(raw))
	}
	if okRes && r.Result == nil {
		r.Result = &null
	}
	if okErr && o["error"] != nil {
		oe := make(map[string]*json.RawMessage)
		if err := json.Unmarshal(*o["error"], &oe); err != nil {
			return errors.New("bad response: " + string(raw))
		}
		if oe["code"] == nil || oe["message"] == nil {
			return errors.New("bad response: " + string(raw))
		}
		if _, ok := oe["data"]; (!ok && len(oe) > 2) || len(oe) > 3 {
			return errors.New("bad response: " + string(raw))
		}
	}
	if o["id"] == nil && !okErr {
		return errors.New("bad response: " + string(raw))
	}

	return nil
}

func (c *clientCodec) handleNotification(r io.Reader) error {
	d := json.NewDecoder(r)
	err := d.Decode(&c.notif)
	// EOF is already handled by ReadResponseHeader
	if err == nil {
		c.receiveNotification()
	}

	return err
}

func (c *clientCodec) receiveNotification() {
	// if we fill the buffer, kill the application
	if len(c.notifications) >= notificationBufferLength {
		out, _ := os.Create("/tmp/goroutine.pprof")
		blockOut, _ := os.Create("/tmp/block.pprof")
		defer out.Close()
		defer blockOut.Close()
		pprof.Lookup("goroutine").WriteTo(out, 2)
		pprof.Lookup("block").WriteTo(blockOut, 2)

		log.Fatal("Stratum client notification buffer is full!  Process will be killed!" +
			"  Read from Client.Notifications to fix this error.")
	}

	c.notifications <- c.notif
}

// Because the stratum connection is bidirectional, we are going to modify the behavior of the client to accept
// notifications from the server (including jobs). Adding some server functionality (receive Notifs) to Client
// seems easier than multiplexing every connection.  Notifications are NOT handled (eg. by RPC svc) by this codec
// This library throws a fatal error if it detects that notifications are not being consumed.
func (c *clientCodec) ReadResponseHeader(r *rpc.Response) error {
	// If return err:
	// - io.EOF will became ErrShutdown or io.ErrUnexpectedEOF
	// - it will be returned as is for all pending calls
	// - client will be shutdown
	// So, return io.EOF as is, return *Error for all other errors.
	b := make([]byte, 0)
	backup := bytes.NewBuffer(b)
	conn := io.TeeReader(c.c, backup)
	d := json.NewDecoder(conn)

	if err := d.Decode(&c.resp); err != nil {
		if err == io.EOF {
			return err
		}
		return c.handleNotification(backup)
	}
	if c.resp.Error != nil {
		return c.resp.Error
	}

	if c.resp.ID == nil {
		// TODO - this is probably the wrong error
		return errInternal
	}

	c.mutex.Lock()
	r.ServiceMethod = c.pending[*c.resp.ID]
	delete(c.pending, *c.resp.ID)
	c.mutex.Unlock()

	r.Error = ""
	r.Seq = *c.resp.ID
	if c.resp.Error != nil {
		r.Error = c.resp.Error.Error()
	}
	return nil
}

func (c *clientCodec) ReadResponseBody(x interface{}) error {
	// If x!=nil and return error e:
	// - this call get e.Error() appended to "reading body "
	// - other pending calls get error as is XXX actually other calls
	//   shouldn't be affected by this error at all, so let's at least
	//   provide different error message for other calls
	if x == nil || c.resp.Result == nil {
		return nil
	}
	if err := json.Unmarshal(*c.resp.Result, x); err != nil {
		e := jsonrpc2.NewError(errInternal.Code, err.Error())
		e.Data = jsonrpc2.NewError(errInternal.Code, "some other Call failed to unmarshal Reply")
		return e
	}
	return nil
}

func (c *clientCodec) Close() error {
	return c.c.Close()
}

type Client struct {
	*rpc.Client
	codec rpc.ClientCodec
}

// Call wraps rpc.Call to provide a timeout - otherwise functionality is the same
func (c *Client) Call(serviceMethod string, args interface{}, reply interface{}) error {
	call := c.Go(serviceMethod, args, reply, nil)
	select {
	case <-call.Done:
		if call.Error != nil {
			return call.Error
		}
		return nil
	case <-time.After(CallTimeout):
		return ErrCallTimedOut
	}
}

// Notify tries to invoke the named function. It return error only in case
// it wasn't able to send request.
func (c *Client) Notify(serviceMethod string, args interface{}) error {
	req := &rpc.Request{
		ServiceMethod: serviceMethod,
		Seq:           seqNotify,
	}
	return c.codec.WriteRequest(req, args)
}

func (c *Client) Notifications() chan Notification {
	return c.codec.(*clientCodec).notifications
}

// NewClient returns a new Client to handle requests to the
// set of services at the other end of the connection.
func NewClient(conn io.ReadWriteCloser) *Client {
	codec := newClientCodec(conn)
	client := rpc.NewClientWithCodec(codec)
	// this is hack around
	_ = client.Go("incrementMySequence", nil, nil, nil)
	return &Client{client, codec}
}

// Dial connects to a JSON-RPC 2.0 server at the specified network address.
func Dial(network, address string) (*Client, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	return NewClient(conn), err
}

// DialTimeout is Dial, but with the timeout specified
func DialTimeout(network, address string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout(network, address, timeout)
	if err != nil {
		return nil, err
	}
	return NewClient(conn), err
}
