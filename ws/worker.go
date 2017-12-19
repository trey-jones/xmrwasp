package ws

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/eyesore/wshandler"
	"github.com/trey-jones/xmrwasp/proxy"
)

const (
	workerTimeout  = 1 * time.Minute
	jobSendTimeout = 30 * time.Second
)

// worker does the work (of mining, well more like accounting) and implements the wshandler.Server interface
type Worker struct {
	wsConn *wshandler.Conn
	id     uint64
	p      *proxy.Proxy

	HashRate    int
	LastJobTime time.Duration
	Difficulty  int

	waitForAuth chan bool
	jobs        chan *proxy.Job
	died        chan bool

	origin string
}

// NewWorker is a wshandler.ServerFactory
func NewWorker() (wshandler.Server, error) {
	w := &Worker{
		waitForAuth: make(chan bool, 1),
		jobs:        make(chan *proxy.Job),
		died:        make(chan bool, 1),
	}

	return w, nil
}

func (w *Worker) Conn() *wshandler.Conn {
	return w.wsConn
}

func (w *Worker) SetConn(c *wshandler.Conn) {
	w.wsConn = c
}

func (w *Worker) OnConnect(r *http.Request) error {
	// if protocols := r.Header.Get("sec-websocket-protocol"); protocols != "" {
	//     protocolList := strings.Split(protocols, ",")
	//     w.Conn().ResponseHeader.Add("sec-websocket-protocol", "json")
	// }
	return nil
}

func (w *Worker) OnOpen() error {
	proxy.GetDirector().Assign(w)

	return nil
}

func (w *Worker) OnMessage(payload []byte, isBinary bool) error {
	// we are assuming all messages are text and json
	f := Frame{}
	err := json.Unmarshal(payload, &f)
	if err != nil {
		return err
	}
	switch f.FrameType {
	case "auth":
		if origin, ok := f.Params["site_key"]; ok {
			w.origin = origin.(string)
		}
		// TODO i guess
		authedMessage := []byte("{\"type\":\"authed\",\"params\":{\"token\":\"\",\"hashes\":0}}")
		w.Conn().Write(authedMessage, false)
		w.waitForAuth <- true
	case "submit":
		w.p.Submit(f.Params)
	}
	return nil
}

func (w *Worker) OnClose(wasClean bool, code int, reason error) error {
	w.died <- true
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

func (w *Worker) Disconnect() {
	w.Conn().Close()
}

// main worker thread - blocks while not receiving; share sending is on message thread
func (w *Worker) Work() {
	timeout := time.NewTimer(workerTimeout)
	select {
	case <-w.waitForAuth:
		timeout.Reset(workerTimeout)
	case <-timeout.C:
		w.died <- true
	}
	for {
		select {
		case <-w.died:
			return
		case job := <-w.jobs:
			frame := NewJobFrame(job)
			data, err := json.Marshal(frame)
			if err != nil {
				zap.S().Error("Error packing job: ", frame)
				zap.S().Error(err)
				break
			}
			w.Conn().Write(data, false)
		}
	}
}

func (w *Worker) NewJob(j *proxy.Job) {
	// spawn a new thread so we don't block anything - we have to get the job to every worker
	go func() {
		timeout := time.NewTimer(jobSendTimeout)
		select {
		case w.jobs <- j:
		case <-timeout.C:
		}
	}()
}

func (w *Worker) expectedHashes() uint32 {
	// TODO - adjustable? does it matter? should it be higher?
	// miners seem to introduce random data anyway...
	return 50000
}

// FRAME types
type Frame struct {
	FrameType string `json:"type"`
	Params    map[string]interface{}
}

type JobFrame struct {
	FrameType string     `json:"type"`
	Params    *proxy.Job `json:"params"`
}

func NewJobFrame(j *proxy.Job) *JobFrame {
	return &JobFrame{"job", j}
}
