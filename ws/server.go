package ws

import (
	"net/http"
	"strconv"

	"github.com/eyesore/ws"
	"github.com/trey-jones/xmrwasp/config"
	"github.com/trey-jones/xmrwasp/logger"
)

func StartServer() {
	h := ws.NewHandler(NewWorker)
	h.AllowAnyOrigin()

	http.Handle("/", h)
	websocketPort := config.Get().WebsocketPort
	portStr := ":" + strconv.Itoa(websocketPort)
	if config.Get().SecureWebsocket {
		logger.Get().Debug("Trying to start secure webserver to handle websocket connections.")
		cert := config.Get().CertFile
		key := config.Get().KeyFile
		err := http.ListenAndServeTLS(portStr, cert, key, nil)
		if err != nil {
			logger.Get().Fatal("Failed to start TLS server: ", err)
		}
		return
	}
	logger.Get().Debug("Starting webserver on port: ", websocketPort)
	err := http.ListenAndServe(portStr, nil)
	if err != nil {
		logger.Get().Fatal("Failed to start server: ", err)
	}
}
