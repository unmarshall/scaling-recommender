package executor

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"unmarshall/scaling-recommender/internal/garden"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/simulation/scenarios/scaledown/simple"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

type Engine interface {
	Run()
	Shutdown()
	GardenAccess() garden.Access
	VirtualControlPlane() virtualenv.ControlPlane
	RecommenderFactory() scaler.Factory
}

type engine struct {
	server              http.Server
	gardenAccess        garden.Access
	virtualControlPlane virtualenv.ControlPlane
	pricingAccess       pricing.InstancePricingAccess
}

func NewExecutor(gardenAccess garden.Access, vControlPlane virtualenv.ControlPlane, pricingAccess pricing.InstancePricingAccess) Engine {
	return &engine{
		server: http.Server{
			Addr: ":8080",
		},
		gardenAccess:        gardenAccess,
		virtualControlPlane: vControlPlane,
		pricingAccess:       pricingAccess,
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

func (e *engine) GardenAccess() garden.Access {
	return e.gardenAccess
}

func (e *engine) VirtualControlPlane() virtualenv.ControlPlane {
	return e.virtualControlPlane
}

func (e *engine) RecommenderFactory() scaler.Factory {
	return scaler.NewFactory(e.virtualControlPlane, e.GardenAccess(), e.pricingAccess)
}

func (e *engine) routes() *http.ServeMux {
	mux := http.NewServeMux()
	simpleScaleDownScenario := simple.New(e)
	mux.HandleFunc("/simulation/scenarios/scaledown/"+simpleScaleDownScenario.Name(), simpleScaleDownScenario.HandlerFn())
	return mux
}
