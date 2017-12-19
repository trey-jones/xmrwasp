package main

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/eyesore/wshandler"
)

const (
	workerTimeout = 2 * time.Minute
)

// worker does the work (of mining, well more like accounting) and implements the wshandler.Server interface
type Worker struct {
	wsConn     *wshandler.Conn
	P          *Proxy
	proxyIndex uint64

	HashRate    int
	LastJobTime time.Duration
	Difficulty  int

	waitForAuth chan bool
	jobs        chan *Job
	died        chan bool

	origin string
}

// NewWorker is a ServerFactory
func NewWorker() (wshandler.Server, error) {
	w := &Worker{
		waitForAuth: make(chan bool, 1),
		jobs:        make(chan *Job),
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
	GetDirector().workers <- w

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
		w.P.submit <- f.Params
	}
	return nil
}

func (w *Worker) OnClose(wasClean bool, code int, reason error) error {
	// much of this code came about in attempt to fix
	// memory leaks and race conditions that ended up being
	// some other problem
	// it can *probably* go away
	w.died <- true
	if w.P != nil {
		w.P.delWorker <- w
		w.P.workerCount--
	}
	<-time.After(30 * time.Second)
	w = nil

	return nil
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
	FrameType string `json:"type"`
	Params    *Job   `json:"params"`
}

func NewJobFrame(j *Job) *JobFrame {
	return &JobFrame{"job", j}
}
