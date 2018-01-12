package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/trey-jones/stratum"
)

var (
	s httptest.Server
)

type mockMiner struct {
	D *websocket.Dialer
	C *websocket.Conn
}

type mockStratumServer struct {
}

func (m *mockStratumServer) run() {
	listener, err := net.Listen("tcp", ":1919")
	if err != nil {
		fmt.Println("Unable to start mock stratum server.")
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting new connection")
			continue
		}
		newsocket(conn)
	}
}

type mockStratumSocket struct {
	c    net.Conn
	send chan []byte
}

func newsocket(c net.Conn) {
	socket := &mockStratumSocket{
		c:    c,
		send: make(chan []byte),
	}
	go socket.doReads()
	go socket.doWrites()
}

func (m *mockStratumSocket) doReads() {
	reader := bufio.NewReader(m.c)
	for {
		rawmessage, err := reader.ReadBytes(stratum.Delimiter)
		if err != nil {
			fmt.Println("Read error: ", err)
			return
		}
		// chop off delimiter
		m.handleMessage(rawmessage[:len(rawmessage)-1])
	}
}

func (m *mockStratumSocket) doWrites() {
	for {
		message := <-m.send
		m.c.Write(append(message, stratum.Delimiter))
	}
}

func (m *mockStratumSocket) handleMessage(msg []byte) {
	r := stratum.Request{}
	err := json.Unmarshal(msg, &r)
	if err != nil {
		fmt.Println("Failed to unmarshal msg : ", string(msg))
		fmt.Println(err)
		return
	}
	switch r.Method {
	case "login":
		m.sendnextjob()
	case "keepalived":
		m.sendkeepalive()
	case "submit":
		m.sendshareresponse()
	case "error":
		m.senderror()
	}
}

func (m *mockStratumSocket) sendloginsuccess() {
	resp := `{"id":1,"jsonrpc":"2.0","result":{"id":"0","job":{"blob":"0606f8f788d1058707a9bdfea5390bdce41ccab6a3c
7e923d3ba32827a0da9771398d9962a5fc80000000063b1df2fb16d38222fe97968b72f0d540277be4f910823e4d66e30b0483c87da04","job_id":"1","target":"b88d0600"},"status":"OK"},"error":null}`
	m.send <- []byte(resp)
}

func (m *mockStratumSocket) sendnextjob() {
	job := `{"jsonrpc":"2.0","method":"job","params":{"blob":"060680f988d105b016222eb249d789b55e393a576541ac8ad81
a16de79ec9a9c0e1ce044ee9feb0000000019895dd9f36fb3babce8b74caef7311199dc8c07b26e21d9f5618a2c94db596d01","job_id":"2","target":"b88d0600"}}`
	m.send <- []byte(job)
}

func (m *mockStratumSocket) sendkeepalive() {
	resp := `{"id":2,"jsonrpc":"2.0","result":{"status":"KEEPALIVED"},"error":null}`
	m.send <- []byte(resp)
}

func (m *mockStratumSocket) sendshareresponse() {
	resp := `{"id":2,"jsonrpc":"2.0","error":null,"result":{"status":"OK"}}`
	m.send <- []byte(resp)
}

func (m *mockStratumSocket) senderror() {
	resp := `{"id":3,"jsonrpc":"2.0","result":null,"error":{"code":-1,"message":"Duplicate share"}}`
	m.send <- []byte(resp)
}

func TestManyWorkers(t *testing.T) {
	howMany := 1000
	for i := 0; i < howMany; i++ {

	}
}

func testConnectSocket() {

}
