package proxy

import (
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/trey-jones/stratum"
	"github.com/trey-jones/xmrwasp/config"
	"github.com/trey-jones/xmrwasp/logger"
)

const (
	// MaxUint protects IDs from overflow if the process runs for thousands of years
	MaxUint = ^uint64(0)

	// TODO adjust - lower means more connections to pool, potentially fewer stales if that is a problem
	maxProxyWorkers = 1024

	retryDelay = 60 * time.Second

	donateCycle time.Duration = 3600 // seconds
	// amount of time to keep the donate connection open after donation ends
	donateShutdownDelay = 30 * time.Second
	donateTimeout       = 10 * time.Second

	keepAliveInterval = 5 * time.Minute
)

var (
	ErrBadJobID       = errors.New("invalid job id")
	ErrDuplicateShare = errors.New("duplicate share")
	ErrMalformedShare = errors.New("malformed share")
)

// Worker does the work for the proxy.  It exposes methods that allow interface with the proxy.
type Worker interface {
	// ID is used to distinguish this worker from other workers on the proxy.
	ID() uint64
	// SetID allows proxies to assign this value when a connection is established.
	SetID(uint64)

	// Workers must implement this method to establish communication with their assigned
	// proxy.  The proxy connection should be stored in order to 1. Submit Shares and 2. Disconnect Cleanly
	SetProxy(*Proxy)
	Proxy() *Proxy

	// Disconnect closes the connection to the proxy from the worker.
	// Ideally it sets up the worker to try and reconnect to a new proxy through the director.
	Disconnect()

	NewJob(*Job)
}

// Proxy manages a group of workers.
type Proxy struct {
	ID       uint64
	SC       *stratum.Client
	DC       *stratum.Client
	SS       *stratum.Server
	director *Director

	authID     string // identifies the proxy to the pool
	aliveSince time.Time
	shares     uint64

	workerCount int

	// workers have to be ID'd so they can be removed when they die
	workerIDs chan uint64
	workers   map[uint64]Worker

	donateInterval time.Duration
	donateLength   time.Duration
	donating       bool
	donateAddr     string

	addWorker chan Worker
	delWorker chan Worker

	submissions chan *share
	donations   chan *share

	notify  chan stratum.Notification
	dnotify chan stratum.Notification // donation jobs

	ready bool

	currentJob *Job
	prevJob    *Job

	donateJob     *Job
	prevDonateJob *Job

	jobMu     sync.Mutex
	jobWaiter *sync.WaitGroup // waits for the first job
}

// New creates a new proxy, starts the work thread, and returns a pointer to it.
func New(id uint64) *Proxy {
	p := &Proxy{
		ID:         id,
		aliveSince: time.Now(),
		workerIDs:  make(chan uint64, 5),
		workers:    make(map[uint64]Worker),

		currentJob:    &Job{},
		prevJob:       &Job{},
		donateJob:     &Job{},
		prevDonateJob: &Job{},

		addWorker: make(chan Worker),
		delWorker: make(chan Worker, 1),

		submissions: make(chan *share),
		donations:   make(chan *share),

		ready:     true,
		donating:  false,
		jobWaiter: &sync.WaitGroup{},
	}
	p.jobWaiter.Add(1)

	ss := stratum.NewServer()
	p.SS = ss
	p.SS.RegisterName("mining", &Mining{})
	logger.Get().Debugln("RPC server is listening on proxy ", p.ID)

	p.configureDonations()

	go p.generateIDs()
	go p.run()
	return p
}

func (p *Proxy) generateIDs() {
	var currentWorkerID uint64

	for {
		currentWorkerID++
		p.workerIDs <- currentWorkerID
	}
}

// nextWorkerID returns the next sequential orderID.  It is safe for concurrent use.
func (p *Proxy) nextWorkerID() uint64 {
	return <-p.workerIDs
}

func (p *Proxy) run() {
	for {
		err := p.login()
		if err == nil {
			break
		}
		logger.Get().Printf("Failed to acquire pool connection.  Retrying in %s.Error: %s\n", retryDelay, err)
		// TODO allow fallback pools here
		<-time.After(retryDelay)
	}

	keepalive := time.NewTicker(keepAliveInterval)
	donateStart := time.NewTimer(p.donateInterval)
	donateEnd := time.NewTimer(p.donateLength)
	donateEnd.Stop() // will be reset after first donate period starts
	defer func() {
		keepalive.Stop()
		p.shutdown()
	}()

	for {
		select {
		// these are from workers
		case s := <-p.submissions:
			err := p.handleSubmit(s, p.SC)
			if err != nil {
				logger.Get().Println("share submission error: ", err)
			}
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "banned") {
				logger.Get().Println("Banned IP - killing proxy: ", p.ID)
				return
			}
		case s := <-p.donations:
			logger.Get().Debugln("donating share for job: ", s.JobID)
			err := p.handleSubmit(s, p.DC) // donate server will handle it's own errors
			if err != nil {
				logger.Get().Println("donate submission error: ", err)
				return
			}
		case w := <-p.addWorker:
			p.receiveWorker(w)
		case w := <-p.delWorker:
			p.removeWorker(w)

		// this comes from the stratum client
		case notif := <-p.notify:
			p.handleNotification(notif, false)
		case notif := <-p.dnotify:
			p.handleNotification(notif, true)

		// these are based on known regular intervals
		case <-donateStart.C:
			// logger.Get().Debugln("Switching to donation server")
			p.donate()
			donateEnd.Reset(p.donateLength)
		case <-donateEnd.C:
			// logger.Get().Debugln("Finished donation cycle. Cleaning up.")
			if p.donating {
				p.undonate()
			}
			donateStart.Reset(p.donateInterval)
		case <-keepalive.C:
			reply := StatusReply{}
			err := p.SC.Call("keepalived", map[string]string{"id": p.authID}, &reply)
			if reply.Error != nil {
				err = reply.Error
			}
			if err != nil {
				logger.Get().Println("Received error from keepalive request: ", err)
				return
			}
			logger.Get().Debugln("Keepalived response: ", reply)
		}
	}
}

func (p *Proxy) donate() {
	// logger.Get().Debugln("Dialing out to: ", p.donateAddr)
	dc, err := stratum.DialTimeout("tcp", p.donateAddr, donateTimeout)
	if err != nil {
		logger.Get().Debugln("failed to connect to donate server")
		return
	}

	params := map[string]interface{}{}
	reply := LoginReply{}
	err = dc.Call("login", params, &reply)
	if reply.Error != nil {
		err = reply.Error
	}
	if err != nil {
		logger.Get().Debugln("failed to login to donate server")
		return
	}

	p.DC = dc
	p.jobMu.Lock()
	p.donating = true
	p.jobMu.Unlock()
	p.dnotify = p.DC.Notifications()

	if err = reply.Job.init(); err != nil {
		logger.Get().Println("bad job from donate login: ", reply.Job, " - err: ", err)
	} else if err = p.handleDonateJob(reply.Job); err != nil {
		logger.Get().Println("error handling new job from donation server: ", err)
	}
}

func (p *Proxy) undonate() {
	p.jobMu.Lock()
	p.donating = false
	p.jobMu.Unlock()
	// give client 30 seconds, then DC
	time.AfterFunc(donateShutdownDelay, func() {
		// logger.Get().Debugln("Shutting down donation conn")
		p.DC.Close()
	})
	p.handleJob(p.currentJob)
}

func (p *Proxy) handleJob(job *Job) (err error) {
	p.jobMu.Lock()
	p.prevJob, p.currentJob = p.currentJob, job
	p.jobMu.Unlock()

	if err != nil || p.donating {
		// logger.Get().Debugln("Skipping regular job broadcast: ", err)
		return
	}

	// logger.Get().Debugln("Broadcasting new regular job: ", job.ID)
	p.broadcastJob()
	return
}

// broadcast a job to all workers
func (p *Proxy) broadcastJob() {
	logger.Get().Debugln("Broadcasting new job to connected workers.")
	for _, w := range p.workers {
		go w.NewJob(p.NextJob())
	}
}

func (p *Proxy) handleDonateJob(job *Job) (err error) {
	// we can use the same mutex here right?
	p.jobMu.Lock()
	if p.donateJob == nil {
		p.donateJob = job
	}
	p.prevDonateJob, p.donateJob = p.donateJob, job
	p.jobMu.Unlock()

	// the donate client will remain connected for ~30s after donate period,
	// so ignore any new jobs at that point
	if err != nil || !p.donating {
		// logger.Get().Debugln("Skipping donate job broadcast: ", err)
		return
	}
	// logger.Get().Debugln("Broadcasting new donate job:", job.ID)
	p.broadcastJob()
	return
}

func (p *Proxy) handleNotification(notif stratum.Notification, donate bool) {
	switch notif.Method {
	case "job":
		job, err := NewJobFromServer(notif.Params.(map[string]interface{}))
		if err != nil {
			logger.Get().Println("bad job: ", notif.Params)
			break
		}
		if !donate {
			err = p.handleJob(job)
		} else {
			err = p.handleDonateJob(job)
		}
		if err != nil {
			// log and wait for the next job?
			logger.Get().Println("error processing job: ", job)
			logger.Get().Println(err)
		}
	default:
		logger.Get().Println("Received notification from server: ",
			"method: ", notif.Method,
			"params: ", notif.Params,
		)
	}
}

func (p *Proxy) login() error {
	sc, err := stratum.Dial("tcp", config.Get().PoolAddr)
	if err != nil {
		return err
	}
	logger.Get().Debugln("Client made pool connection.")
	p.SC = sc

	p.notify = p.SC.Notifications()
	params := map[string]interface{}{
		"login": config.Get().PoolLogin,
		"pass":  config.Get().PoolPassword,
	}
	reply := LoginReply{}
	err = p.SC.Call("login", params, &reply)
	if err != nil {
		return err
	}
	logger.Get().Debugln("Successfully logged into pool.")
	p.authID = reply.ID
	if err = reply.Job.init(); err != nil {
		logger.Get().Println("bad job from login: ", reply.Job, "- err: ", err)
		// still just wait for the next job
	} else if err = p.handleJob(reply.Job); err != nil {
		logger.Get().Println("Error processing job: ", reply.Job)
		// continue and just wait for the next job?
		// this shouldn't happen
	}

	logger.Get().Printf("****    Connected and logged in to pool server: %s \n", config.Get().PoolAddr)
	logger.Get().Println("****    Broadcasting jobs to workers.")

	// now we have a job, so release jobs
	p.jobWaiter.Done()

	return nil
}

func (p *Proxy) validateShare(s *share) error {
	var job *Job
	switch {
	case s.JobID == p.currentJob.ID:
		job = p.currentJob
	case s.JobID == p.prevJob.ID:
		job = p.prevJob
	case s.JobID == p.donateJob.ID:
		job = p.donateJob
	case s.JobID == p.prevDonateJob.ID:
		job = p.prevDonateJob
	default:
		return ErrBadJobID
	}
	// for _, n := range job.submittedNonces {
	// 	if n == s.Nonce {
	// 		return ErrDuplicateShare
	// 	}
	// }
	return s.validate(job)
}

func (p *Proxy) receiveWorker(w Worker) {
	p.workers[w.ID()] = w
	p.workerCount++
}

func (p *Proxy) removeWorker(w Worker) {
	delete(p.workers, w.ID())
	p.workerCount--
	// potentially check for len(workers) == 0, start timer to spin down proxy if empty
	// like apache, we might expire a proxy at some point anyway, just to try and reclaim potential resources
	// in workers map, avert id overflow, etc.
}

func (p *Proxy) configureDonations() {
	p.donateAddr = "donate.xmrwasp.com:3333"
	// p.donateAddr = "localhost:13334"
	donateLevel := config.Get().DonateLevel
	if donateLevel <= 0 {
		donateLevel = 1
	}
	p.donateLength = (time.Duration(math.Floor(float64(donateCycle)*(float64(donateLevel)/100))) * time.Second)
	p.donateInterval = (donateCycle * time.Second) - p.donateLength
	// logger.Get().Debugln("DonateLength is: ", p.donateLength)
	// logger.Get().Debugln("DonateInterval is: ", p.donateInterval)
}

func (p *Proxy) shutdown() {
	// kill worker connections - they should connect to a new proxy if active
	p.ready = false
	for _, w := range p.workers {
		w.Disconnect()
	}
	<-time.After(1 * time.Minute)
	p.director.removeProxy(p)
}

func (p *Proxy) isReady() bool {
	// the worker count read is a race TODO
	return p.ready && p.workerCount < maxProxyWorkers
}

func (p *Proxy) handleSubmit(s *share, c *stratum.Client) (err error) {
	defer func() {
		close(s.Response)
		close(s.Error)
	}()
	if c == nil {
		logger.Get().Println("dropping share due to nil client for job: ", s.JobID)
		err = errors.New("no client to handle share")
		s.Error <- err
		return
	}

	if err = p.validateShare(s); err != nil {
		logger.Get().Debug("share: ", s)
		logger.Get().Println("rejecting share with: ", err)
		s.Error <- err
		return
	}

	s.AuthID = p.authID
	reply := StatusReply{}
	if err = c.Call("submit", s, &reply); err != nil {
		s.Error <- err
		return
	}
	if reply.Status == "OK" {
		p.shares++
	}

	// logger.Get().Debugf("proxy %v share submit response: %s", p.ID, reply)
	s.Response <- &reply
	s.Error <- nil
	return
}

// Submit sends worker shares to the pool.  Safe for concurrent use.
func (p *Proxy) Submit(params map[string]interface{}) (*StatusReply, error) {
	s := newShare(params)

	if s.JobID == "" {
		return nil, ErrBadJobID
	}
	if s.Nonce == "" {
		return nil, ErrMalformedShare
	}

	// if it matters - locking jobMu should be fine
	// there might be a race for the job ids's but it shouldn't matter
	if s.JobID == p.currentJob.ID || s.JobID == p.prevJob.ID {
		p.submissions <- s
	} else if s.JobID == p.donateJob.ID || s.JobID == p.prevDonateJob.ID {
		p.donations <- s
	} else {
		return nil, ErrBadJobID
	}

	return <-s.Response, <-s.Error
}

// NextJob gets gets the next job (on the current block) and increments the nonce
func (p *Proxy) NextJob() *Job {
	p.jobWaiter.Wait() // only waits for first job from login
	p.jobMu.Lock()
	defer p.jobMu.Unlock()
	var j *Job
	if !p.donating {
		j = p.currentJob.Next()
	} else {
		j = p.donateJob.Next()
	}

	return j
}

// Add a worker to the proxy - safe for concurrent use.
func (p *Proxy) Add(w Worker) {
	w.SetProxy(p)
	w.SetID(p.nextWorkerID())

	p.addWorker <- w
}

// Remove a worker from the proxy - safe for concurrent use.
func (p *Proxy) Remove(w Worker) {
	p.delWorker <- w
}
