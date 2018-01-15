package tcp

import (
	"context"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/trey-jones/stratum"
	"github.com/trey-jones/xmrwasp/proxy"
)

const (
	workerTimeout  = 1 * time.Minute
	jobSendTimeout = 30 * time.Second
)

// worker does the work (of mining, well more like accounting)
type Worker struct {
	conn net.Conn
	id   uint64
	p    *proxy.Proxy

	// codec will be used directly for sending jobs
	// this is not ideal, and it would be nice to do this differently
	codec *stratum.DefaultServerCodec

	jobs chan *proxy.Job
}

// SpawnWorker spawns a new TCP worker and adds it to a proxy
func SpawnWorker(conn net.Conn) {
	w := &Worker{
		conn: conn,
		jobs: make(chan *proxy.Job),
	}
	ctx := context.WithValue(context.Background(), "worker", w)
	codec := stratum.NewDefaultServerCodecContext(ctx, w.Conn())
	w.codec = codec.(*stratum.DefaultServerCodec)

	p := proxy.GetDirector().NextProxy()
	p.Add(w)

	// blocks until disconnect
	w.Proxy().SS.ServeCodec(codec)

	w.p.Remove(w)
}

func (w *Worker) Conn() net.Conn {
	return w.conn
}

func (w *Worker) SetConn(c net.Conn) {
	w.conn = c
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
	w.Conn().Close()
}

func (w *Worker) NewJob(j *proxy.Job) {
	err := w.codec.Notify("job", j)
	if err != nil {
		zap.S().Error("Error sending job to worker: ", err)
		w.Disconnect()
	}
	// other actions? shut down worker?
}

func (w *Worker) expectedHashes() uint32 {
	// this is a complete unknown at this time.
	return 0x7a120
}
