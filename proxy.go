package main

import (
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/trey-jones/stratum"
)

const (
	// clock over after hitting this value to prevent overflow
	MaxUint = ^uint64(0) - 1000

	// TODO - worker computes expected hashes
	expectedHashes = 50000
)

var (
	keepAliveInterval = 5 * time.Minute

	ErrBadJobID       = errors.New("Submitted Job ID is not longer valid.")
	ErrDuplicateShare = errors.New("Share has already been submitted for current job.")
	ErrMalformedShare = errors.New("Share is missing required data.")
)

// proxy manages a group of workers.
type Proxy struct {
	ID       uint64
	SC       *stratum.Client
	director *Director

	aliveSince time.Time
	shares     uint64

	workerCount int
	// workers have to be ID'd so they can be removed when they die
	currentWorkerID uint64
	workers         map[uint64]*Worker
	HashRate        int

	addWorker chan *Worker
	delWorker chan *Worker
	submit    chan map[string]interface{}

	ready        bool
	currentJob   *Job
	currentBlob  []byte
	currentNonce uint32
}

func NewProxy(id uint64) *Proxy {
	p := &Proxy{
		ID:         id,
		aliveSince: time.Now(),
		workers:    make(map[uint64]*Worker),
		addWorker:  make(chan *Worker),
		delWorker:  make(chan *Worker, 1),
		submit:     make(chan map[string]interface{}),
		ready:      true,
	}
	sc, err := stratum.NewClient(Config().PoolAddr, zap.S())
	if err != nil {
		// fallback, retry, anything!
		zap.S().Fatal("Unable to connect to remote server")
	}
	p.SC = sc

	go p.Run()
	return p
}

func (p *Proxy) Ready() bool {
	// once we fill up, make a new proxy - not sure if this is the best idea, or not
	return p.ready && p.workerCount < 2048
}

func (p *Proxy) Add(w *Worker) {
	w.P = p
	go w.Work()
	p.addWorker <- w
	p.workerCount++
}

func (p *Proxy) Run() {
	p.login()
	// login blocks until first job is received
	keepalive := time.NewTicker(keepAliveInterval)
	defer keepalive.Stop()
	defer p.shutdown()
	for {
		select {
		// these are from workers
		case w := <-p.addWorker:
			p.receiveWorker(w)
		case share := <-p.submit:
			if err := p.validateShare(share); err != nil {
				zap.S().Info("Rejecting share: ", share)
				zap.S().Info("Reason: ", err)
				break
			}
			submitRequest := stratum.Request{
				Method: "submit",
				Params: share,
			}
			p.SC.Requests <- &submitRequest
			// test dup share
			// <-time.After(5 * time.Second)
			// p.SC.Requests <- &submitRequest
		case w := <-p.delWorker:
			p.removeWorker(w)

		// these are from  the client
		case resp := <-p.SC.Received:
			if status, ok := resp.Result["status"]; ok && status == "OK" {
				zap.S().Info("Share accepted on proxy ", p.ID)
				p.shares++
			}
			// TODO notify worker of accepted
			// case notif := <-p.SC.Notifications:
		case params := <-p.SC.Jobs:
			job := NewJobFromServer(params)
			err := p.Handle(job)
			if err != nil {
				// log and wait for the next job?
				zap.S().Error("Error processing job: ", job)
				zap.S().Error(err)
			}
		case err := <-p.SC.Errors:
			zap.S().Error("got err: ", err)
			// all errors seem to be fatal, resulting in worker ban or server gone
			// so wekill the proxy and try to connect with a new one
			return
		case <-keepalive.C:
			keepaliveRequest := stratum.Request{
				Method: "keepalived",
				Params: make(map[string]interface{}),
			}
			p.SC.Requests <- &keepaliveRequest
		}

	}
}

func (p *Proxy) Handle(job *Job) (err error) {
	p.currentJob = job
	p.currentNonce, p.currentBlob, err = job.Nonce()
	if err != nil {
		return
	}
	for _, w := range p.workers {
		p.sendJob(w)
	}
	return
}

func (p *Proxy) login() {
	loginRequest := stratum.Request{
		Method: "login",
		Params: map[string]interface{}{
			"login": Config().PoolLogin,
			"pass":  Config().PoolPassword,
		},
	}
	p.SC.Requests <- &loginRequest
	select {
	case params := <-p.SC.Jobs:
		job := NewJobFromServer(params)
		err := p.Handle(job)
		if err != nil {
			// log and wait for the next job?
			zap.S().Error("Error processing job: ", job)
			zap.S().Error(err)
		}
	case err := <-p.SC.Errors:
		zap.S().Error("Login error received for proxy ", p.ID, ": ", err)
		p.shutdown()
		return
	case <-time.After(30 * time.Second):
	}
}

func (p *Proxy) validateShare(s map[string]interface{}) error {
	if jobID, ok := s["job_id"]; !ok || jobID.(string) != p.currentJob.ID {
		return ErrBadJobID
	}
	inonce, ok := s["nonce"]
	if !ok {
		return ErrMalformedShare
	}
	nonce := inonce.(string)
	for _, n := range p.currentJob.SubmittedNonces {
		if n == nonce {
			return ErrDuplicateShare
		}
	}
	p.currentJob.SubmittedNonces = append(p.currentJob.SubmittedNonces, nonce)

	return nil
}

func (p *Proxy) sendJob(w *Worker) {
	j := NewJob(p.currentBlob, p.currentNonce, p.currentJob.ID, p.currentJob.Target)
	go func() {
		timeout := time.NewTimer(30 * time.Second)
		select {
		case w.jobs <- j:
		case <-timeout.C:
		}
	}()
	p.currentNonce += expectedHashes
}

func (p *Proxy) receiveWorker(w *Worker) {
	index := p.nextWorkerID()
	p.workers[index] = w
	w.proxyIndex = index

	if p.currentJob != nil {
		p.sendJob(w)
	}
}

func (p *Proxy) removeWorker(w *Worker) {
	delete(p.workers, w.proxyIndex)
}

func (p *Proxy) nextWorkerID() uint64 {
	if p.currentWorkerID >= MaxUint {
		p.currentWorkerID = 0
	}
	p.currentWorkerID++
	return p.currentWorkerID
}

func (p *Proxy) shutdown() {
	// kill worker connections - they should connect to a new proxy if active
	p.ready = false
	for _, w := range p.workers {
		w.Conn().Close()
	}
	p.director.removeProxy(p)
}
