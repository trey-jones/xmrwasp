package proxy

import (
	"flag"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	ews "github.com/eyesore/ws"
	"github.com/gorilla/websocket"
	"github.com/trey-jones/stratum"
	"github.com/trey-jones/wstest"
	"github.com/trey-jones/xmrwasp/logger"
	"github.com/trey-jones/xmrwasp/tcp"
	"github.com/trey-jones/xmrwasp/ws"

	"net/http/httptest"
	_ "net/http/pprof"
)

const (
	mockPoolURL   = "localhost:13333"
	mockDonateURL = "localhost:13334"

	blockInterval      = 2 * time.Minute
	submitMax      int = 1000 // seconds
	workerSpawnMax int = 500  // milleseconds
	disconnectMax      = 2400 // seconds
)

var (
	testWsServer *httptest.Server
	// h               http.Handler
	mockPoolReady   = make(chan bool, 1)
	donatePoolReady = make(chan bool, 1)

	// cancel channels for individual threads
	endWsTest  chan bool
	endTCPTest chan bool

	// command line args
	simDuration int // minutes
	maxWorkers  int
	simMode     string
	debug       bool
)

func randomJobID() string {
	fakeJobID := rand.Intn(99999999)

	return strconv.Itoa(fakeJobID)
}

type MockPool struct{}

func (m *MockPool) Login(p map[string]interface{}, resp *LoginReply) error {
	resp.ID = "0"
	resp.Job = &Job{
		Blob:   "0606f8f788d1058707a9bdfea5390bdce41ccab6a3c7e923d3ba32827a0da9771398d9962a5fc80000000063b1df2fb16d38222fe97968b72f0d540277be4f910823e4d66e30b0483c87da04",
		ID:     randomJobID(),
		Target: "notarealtarget",
	}
	resp.Status = "OK"
	return nil
}

func (m *MockPool) Submit(p map[string]interface{}, resp *StatusReply) error {
	resp.Status = "OK"
	return nil
}

func (m *MockPool) Keepalived(p map[string]interface{}, resp *StatusReply) error {
	resp.Status = "KEEPALIVED"
	return nil
}

type wsClient struct {
	c       *websocket.Conn
	jobID   string
	outbox  chan interface{}
	jobIDMu sync.Mutex
}

func newWsClient(t *testing.T) error {
	// using the test server throws "Too many open files" on mac - wstest seems to work ok and spins up workers faster
	// url := strings.Replace(testWsServer.URL, "http", "ws", 1)
	h := ews.NewHandler(ws.NewWorker)
	d := wstest.NewDialer(h, nil)
	// conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	conn, resp, err := d.Dial("ws://notarealserver", nil)
	if err != nil {
		t.Fatal("Failed to spawn WS worker: ", err)
	}
	if got, want := resp.StatusCode, http.StatusSwitchingProtocols; got != want {
		t.Fatalf("resp.StatusCode = %q, want %q", got, want)
	}

	client := &wsClient{
		c:      conn,
		jobID:  "",
		outbox: make(chan interface{}),
	}
	go client.simulate(t)

	return nil
}

func (w *wsClient) simulate(t *testing.T) {
	go w.readLoop(t)
	go w.writeLoop(t)
	nextSubmit := nextRandomSubmit()
	disconnectIn := time.Duration(rand.Intn(disconnectMax))
	submitTime := time.NewTimer(nextSubmit * time.Second)
	disconnectTime := time.NewTimer(disconnectIn * time.Second)
	defer submitTime.Stop()

	w.sendAuth()
	// should we validate jobs received?
	for {
		select {
		case <-submitTime.C:
			w.sendSubmit()
			nextSubmit = nextRandomSubmit()
			submitTime = time.NewTimer(nextSubmit * time.Second)
		case <-disconnectTime.C:
			w.c.Close()
			close(w.outbox)
			return
		}
	}
}

func (w *wsClient) sendAuth() {
	authRequest := map[string]interface{}{
		"type": "auth",
		"params": map[string]interface{}{
			"something":     "",
			"somethingelse": "",
		},
	}
	w.outbox <- authRequest
}

func (w *wsClient) sendSubmit() {
	// limit number of duplicate shares
	randomNonce := rand.Intn(99999999999)
	w.jobIDMu.Lock()
	submitRequest := map[string]interface{}{
		"type": "submit",
		"params": map[string]interface{}{
			"job_id": w.jobID,
			"nonce":  strconv.Itoa(randomNonce),
			"result": "not the actual result",
		},
	}
	w.jobIDMu.Unlock()
	w.outbox <- submitRequest
}

func (w *wsClient) readLoop(t *testing.T) {
	for {
		// check if it's a job and if it is, update job id
		msg := make(map[string]interface{})
		err := w.c.ReadJSON(&msg)
		if err != nil {
			break
		}
		if t, ok := msg["type"]; !ok || t != "job" {
			continue
		}
		job := msg["params"].(map[string]interface{})
		w.jobIDMu.Lock()
		w.jobID = job["job_id"].(string)
		w.jobIDMu.Unlock()
	}
}

func (w *wsClient) writeLoop(t *testing.T) {
	for message := range w.outbox {
		err := w.c.WriteJSON(message)
		if err != nil {
			break
		}
	}
}

type tcpClient struct {
	jobID   string
	notify  chan stratum.Notification
	c       *stratum.Client
	jobIDMu sync.Mutex
}

func newTCPClient(t *testing.T) error {
	server, client := net.Pipe()
	c := &tcpClient{
		c: stratum.NewClient(client),
	}
	c.notify = c.c.Notifications()
	go tcp.SpawnWorker(server)
	// hack around race for now
	time.Sleep(250 * time.Millisecond)
	go c.simulate(t)

	return nil
}

func (c *tcpClient) simulate(t *testing.T) {
	nextSubmit := nextRandomSubmit()
	disconnectIn := time.Duration(rand.Intn(disconnectMax))
	submitTime := time.NewTimer(nextSubmit * time.Second)
	disconnectTime := time.NewTimer(disconnectIn * time.Second)
	defer submitTime.Stop()

	err := c.sendAuth()
	if err != nil {
		t.Fatal("TCP worker was unable to login: ", err)
		return
	}

	for {
		select {
		case notif := <-c.notify:
			if notif.Method == "job" {
				job := NewJobFromServer(notif.Params.(map[string]interface{}))
				c.jobIDMu.Lock()
				c.jobID = job.ID
				c.jobIDMu.Unlock()
			}
		case <-submitTime.C:
			err := c.sendSubmit()
			if err != nil {
				t.Fatal("TCP worker got bad response on share submission: ", err)
			}
			nextSubmit = nextRandomSubmit()
			submitTime = time.NewTimer(nextSubmit * time.Second)
		case <-disconnectTime.C:
			c.c.Close()
			return
		}
	}
}

func (c *tcpClient) sendAuth() error {
	loginReply := LoginReply{}
	err := c.c.Call("login", map[string]interface{}{}, &loginReply)
	if err != nil {
		return err
	}
	if loginReply.Error != nil || loginReply.Status != "OK" {
		return loginReply.Error
	}

	c.jobID = loginReply.Job.ID
	return nil
}

func (c *tcpClient) sendSubmit() error {
	randomNonce := rand.Intn(99999999999)
	c.jobIDMu.Lock()
	params := map[string]interface{}{
		"job_id": c.jobID,
		"nonce":  strconv.Itoa(randomNonce),
		"result": "does not matter",
	}
	c.jobIDMu.Unlock()
	reply := StatusReply{}
	err := c.c.Call("submit", params, &reply)
	if err != nil {
		return err
	}
	if reply.Error != nil {
		return reply.Error
	}
	return nil
}

func nextRandomSubmit() time.Duration {
	return time.Duration(rand.Intn(submitMax))
}

func startMockPool(t *testing.T) {
	listener, err := net.Listen("tcp", mockPoolURL)
	if err != nil {
		t.Fatal("Unable to start mock pool: ", err)
	}
	defer listener.Close()
	s := stratum.NewServer()
	codecs := make([]*stratum.DefaultServerCodec, 0)
	go broadcastJobs(&codecs)
	s.RegisterName("mining", &MockPool{})
	close(mockPoolReady)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Failed to accept new proxy connection: ", err)
			continue
		}
		codec := stratum.NewDefaultServerCodec(conn)
		codecs = append(codecs, codec.(*stratum.DefaultServerCodec))
		go s.ServeCodec(codec)
	}
}

func startDonatePool(t *testing.T) {
	listener, err := net.Listen("tcp", mockDonateURL)
	if err != nil {
		t.Fatal("Unable to start donate pool: ", err)
	}
	defer listener.Close()
	s := stratum.NewServer()
	codecs := make([]*stratum.DefaultServerCodec, 0)
	go broadcastJobs(&codecs)
	s.RegisterName("mining", &MockPool{})
	close(donatePoolReady)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Failed to accept new proxy connection: ", err)
			continue
		}
		codec := stratum.NewDefaultServerCodec(conn)
		codecs = append(codecs, codec.(*stratum.DefaultServerCodec))
		go s.ServeCodec(codec)
	}
}

func startMockWebserver(t *testing.T) {
	h := ews.NewHandler(ws.NewWorker)
	testWsServer = httptest.NewServer(h)
}

// broadcast job always sends the same job
func broadcastJobs(clients *[]*stratum.DefaultServerCodec) {
	jobSender := time.NewTicker(blockInterval)
	defer jobSender.Stop()
	for {
		<-jobSender.C
		for _, c := range *clients {
			fakeJob := &Job{
				ID: randomJobID(),
				// fake, but valid
				Blob:   "0707f8f788d1058707a9bdfea5390bdce41ccab6a3c7e923d3ba32827a0da9771398d9962a5fc80000000063b1df2fb16d38222fe97968b72f0d540277be4f910823e4d66e30b0483c87da04",
				Target: "faketarget",
			}
			err := c.Notify("job", fakeJob)
			if err != nil {
				// this would throw for each time the donation client has connected and disconnected
				// no big deal
				// log.Println("Unable to notify client of job: ", err)
			}
		}
	}
}

func setEnv() {
	os.Setenv("XMRWASP_LOGIN", "testwallet")
	os.Setenv("XMRWASP_PASSWORD", "x")
	os.Setenv("XMRWASP_URL", mockPoolURL)
	os.Setenv("XMRWASP_DONATE", "98")
}

func configure() {
	// right now workers means "workers of each type, not simulaneous"
	flag.IntVar(&maxWorkers, "workers", 1000, "max total number of workers of each type to spawn during the simulation")
	// TO increase beyond 10, timeout flag must also be present and greater than duration
	flag.IntVar(&simDuration, "duration", 9, "number of minutes to run the simulation")
	flag.StringVar(&simMode, "mode", "all", "which sims to run. valid values: ws, tcp, all")
	flag.BoolVar(&debug, "d", false, "use the debug logger during the test (can be very verbose")
	flag.Parse()

	lc := &logger.Config{W: nil}
	if debug {
		lc.Level = logger.Debug
	}
	logger.Configure(lc)
	logger.Get().Debug("Logger is configured")
}

func TestMain(m *testing.M) {
	setEnv()
	defer os.Clearenv()

	configure()

	// serve pprof data during simulation
	runtime.SetBlockProfileRate(1)
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	rand.Seed(time.Now().Unix())

	os.Exit(m.Run())
}

func TestSimulate(t *testing.T) {
	// webserver only needed for WS, but whatever
	startMockWebserver(t)
	defer testWsServer.Close()
	go startMockPool(t)
	go startDonatePool(t)
	<-mockPoolReady
	<-donatePoolReady
	endTest := time.NewTimer(time.Duration(simDuration) * time.Minute)
	defer endTest.Stop()
	endWsTest = make(chan bool, 1)
	endTCPTest = make(chan bool, 1)

	switch simMode {
	case "ws":
		go testWsWorkers(t)
	case "tcp":
		go testTCPWorkers(t)
	default:
		testAllWorkerTypes(t)
	}

	<-endTest.C

	if simMode != "tcp" {
		endWsTest <- true
	}
	if simMode != "ws" {
		endTCPTest <- true
	}
	return
}

func testAllWorkerTypes(t *testing.T) {
	go testWsWorkers(t)
	go testTCPWorkers(t)
}

func testWsWorkers(t *testing.T) {
	workerSpawner := time.NewTimer(1 * time.Second)
	defer workerSpawner.Stop()
	for i := 0; i < maxWorkers; i++ {
		select {
		case <-workerSpawner.C:
			err := newWsClient(t)
			if err != nil {
				t.Fatal("Failed to spawn a websocket worker: ", err)
			}
			nextWorker := time.Duration(rand.Intn(workerSpawnMax))
			workerSpawner = time.NewTimer(nextWorker * time.Millisecond)
		case <-endWsTest:
			return
		}
	}
	logger.Get().Debugln("WS worker loop finished.  Waiting on test duration.")
}

func testTCPWorkers(t *testing.T) {
	workerSpawner := time.NewTimer(1 * time.Second)
	defer workerSpawner.Stop()
	for i := 0; i < maxWorkers; i++ {
		select {
		case <-workerSpawner.C:
			err := newTCPClient(t)
			if err != nil {
				t.Fatal("Failed to spawn a TCP worker: ", err)
			}
			nextWorker := time.Duration(rand.Intn(workerSpawnMax))
			workerSpawner = time.NewTimer(nextWorker * time.Millisecond)
		case <-endTCPTest:
			return
		}
	}
	logger.Get().Debugln("TCP worker loop finished. Waiting on test duration.")
}
