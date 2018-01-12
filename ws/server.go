package ws

import (
	"net/http"

	"github.com/eyesore/wshandler"
	"github.com/trey-jones/xmrwasp/config"
	"go.uber.org/zap"
)

func StartServer(logger *zap.SugaredLogger) {
	h := wshandler.NewConnector(NewWorker)
	h.AllowAnyOrigin()

	http.Handle("/", h)
	websocketPort := ":" + config.Get().WebsocketPort
	logger.Debug("Starting webserver on port: ", websocketPort)
	err := http.ListenAndServe(websocketPort, nil)
	if err != nil {
		logger.Fatal("Failed to start server")
	}
}
