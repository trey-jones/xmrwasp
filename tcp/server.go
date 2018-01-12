package tcp

import (
	"net"

	"github.com/trey-jones/xmrwasp/config"
	"go.uber.org/zap"
)

func StartServer(logger *zap.SugaredLogger) {
	tcpPort := ":" + config.Get().StratumPort
	logger.Debug("Starting TCP listener on port: ", tcpPort)
	listener, err := net.Listen("tcp", tcpPort)
	if err != nil {
		zap.S().Error("Unable to listen for tcp connections on port: ", listener.Addr(),
			" Listen failed with error: ", err)
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			zap.S().Error("Unable to accept connection: ", err)
		}
		go SpawnWorker(conn)
	}
}
