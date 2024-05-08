package simulation

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/garden"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

type Engine interface {
	Run()
	Shutdown()
	GardenAccess() garden.Access
	VirtualControlPlane() virtualenv.ControlPlane
	PricingAccess() pricing.InstancePricingAccess
	RecommenderFactory() scaler.Factory
	TargetShootCoordinate() common.ShootCoordinate
}

type engine struct {
	server              http.Server
	gardenAccess        garden.Access
	virtualControlPlane virtualenv.ControlPlane
	pricingAccess       pricing.InstancePricingAccess
	targetShootCoord    *common.ShootCoordinate
}

func NewExecutor(gardenAccess garden.Access, vControlPlane virtualenv.ControlPlane, pricingAccess pricing.InstancePricingAccess, targetShootCoord *common.ShootCoordinate) Engine {
	return &engine{
		server: http.Server{
			Addr: ":8080",
		},
		gardenAccess:        gardenAccess,
		virtualControlPlane: vControlPlane,
		pricingAccess:       pricingAccess,
		targetShootCoord:    targetShootCoord,
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

func (e *engine) PricingAccess() pricing.InstancePricingAccess {
	return e.pricingAccess
}

func (e *engine) RecommenderFactory() scaler.Factory {
	return scaler.NewFactory(e.virtualControlPlane, e.pricingAccess)
}

func (e *engine) TargetShootCoordinate() common.ShootCoordinate {
	return *e.targetShootCoord
}

func (e *engine) routes() *http.ServeMux {
	mux := http.NewServeMux()
	h := Handler{engine: e}
	mux.HandleFunc("POST /simulation/", h.run)
	return mux
}
