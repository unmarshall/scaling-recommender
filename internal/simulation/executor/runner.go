package executor

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"unmarshall/scaling-recommender/internal/simulation/scenarios/scaledown/simple"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

type Engine interface {
	Run()
	Shutdown()
}

type engine struct {
	server              http.Server
	virtualControlPlane virtualenv.ControlPlane
}

func NewExecutor(vControlPlane virtualenv.ControlPlane) Engine {
	return &engine{
		server: http.Server{
			Addr: ":8080",
		},
		virtualControlPlane: vControlPlane,
	}
}

func (e *engine) Run() {
	e.server.Handler = e.routes()
	if err := e.server.ListenAndServe(); err != nil {
		slog.Error("error starting scenario http server", "error", err)
		os.Exit(1)
	}
}

func (e *engine) Shutdown() {
	if err := e.server.Shutdown(context.Background()); err != nil {
		slog.Error("error shutting down scenario http server", "error", err)
	}
}

func (e *engine) routes() *http.ServeMux {
	mux := http.NewServeMux()
	simpleScaleDownScenario := simple.New(e)
	mux.HandleFunc("/simulation/scenarios/scaledown/"+simpleScaleDownScenario.Name(), simpleScaleDownScenario.HandlerFn())
	return mux
}
