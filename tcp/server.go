package tcp

import (
	"net"

	"github.com/trey-jones/xmrwasp/config"
	"github.com/trey-jones/xmrwasp/logger"
)

func StartServer() {
	tcpPort := ":" + config.Get().StratumPort
	logger.Get().Debug("Starting TCP listener on port: ", tcpPort)
	listener, err := net.Listen("tcp", tcpPort)
	if err != nil {
		logger.Get().Fatal("Unable to listen for tcp connections on port: ", listener.Addr(),
			" Listen failed with error: ", err)
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Get().Println("Unable to accept connection: ", err)
		}
		go SpawnWorker(conn)
	}
}
