package ws

import (
	"net/http"

	"github.com/eyesore/ws"
	"github.com/trey-jones/xmrwasp/config"
	"github.com/trey-jones/xmrwasp/logger"
)

func StartServer() {
	h := ws.NewHandler(NewWorker)
	h.AllowAnyOrigin()

	http.Handle("/", h)
	websocketPort := ":" + config.Get().WebsocketPort
	logger.Get().Debug("Starting webserver on port: ", websocketPort)
	err := http.ListenAndServe(websocketPort, nil)
	if err != nil {
		logger.Get().Fatal("Failed to start server")
	}
}
