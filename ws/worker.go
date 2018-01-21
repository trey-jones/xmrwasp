package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/eyesore/ws"
	"github.com/trey-jones/stratum"
	"github.com/trey-jones/xmrwasp/proxy"
)

const (
	workerTimeout  = 1 * time.Minute
	jobSendTimeout = 30 * time.Second
)

// Worker does the work (of mining, well more like accounting) and implements the ws.Server interface
type Worker struct {
	wsConn *ws.Conn
	id     uint64
	p      *proxy.Proxy

	// codec will be used directly for sending jobs
	// this is not ideal, and it would be nice to do this differently
	codec *stratum.CoinhiveServerCodec

	jobs chan *proxy.Job
}

// NewWorker is a ws.Factory
func NewWorker() (ws.Connector, error) {
	w := &Worker{
		jobs: make(chan *proxy.Job),
	}

	return w, nil
}

// Conn implements ews.Connector
func (w *Worker) Conn() *ws.Conn {
	return w.wsConn
}

// SetConn implements ews.Connector
func (w *Worker) SetConn(c *ws.Conn) {
	w.wsConn = c
}

// OnConnect implements ews.Connector
func (w *Worker) OnConnect(r *http.Request) error {
	// if protocols := r.Header.Get("sec-websocket-protocol"); protocols != "" {
	//     protocolList := strings.Split(protocols, ",")
	//     w.Conn().ResponseHeader.Add("sec-websocket-protocol", "json")
	// }
	return nil
}

// OnOpen implements ews.Connector
func (w *Worker) OnOpen() error {
	ctx := context.WithValue(context.Background(), "worker", w)
	codec := stratum.NewCoinhiveServerCodecContext(ctx, w.Conn())
	w.codec = codec.(*stratum.CoinhiveServerCodec)

	p := proxy.GetDirector().NextProxy()
	p.Add(w)
	go w.Proxy().SS.ServeCodec(codec)

	return nil
}

// OnClose implements ews.Connector
func (w *Worker) OnClose(wasClean bool, code int, reason error) error {
	// logger.Get().Debugln("OnClose is called for worker")
	w.p.Remove(w)

	return nil
}

// Worker interface
func (w *Worker) ID() uint64 {
	return w.id
}

func (w *Worker) SetID(i uint64) {
	w.id = i
}

func (w *Worker) SetProxy(p *proxy.Proxy) {
	w.p = p
}

func (w *Worker) Proxy() *proxy.Proxy {
	return w.p
}

func (w *Worker) Disconnect() {
	// logger.Get().Debugln("Disconnect is called for worker.")
	w.Conn().Close()
}

func (w *Worker) NewJob(j *proxy.Job) {
	err := w.codec.Notify("job", j)
	if err != nil {
		// logger.Get().Debugln("Error sending job to worker: ", err)
		w.Disconnect()
	}
}

// unused
func (w *Worker) expectedHashes() uint32 {
	// TODO - adjustable? does it matter? should it be higher?
	// miners seem to introduce random data anyway...
	return 0x7a120
}
