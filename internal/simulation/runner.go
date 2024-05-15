package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"unmarshall/scaling-recommender/internal/app"
	"unmarshall/scaling-recommender/internal/scaler/factory"

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
	TargetShootCoordinate() app.ShootCoordinate
}

type engine struct {
	server              http.Server
	gardenAccess        garden.Access
	virtualControlPlane virtualenv.ControlPlane
	pricingAccess       pricing.InstancePricingAccess
	targetShootCoord    *app.ShootCoordinate
	recommenderFactory  scaler.Factory
}

func NewExecutor(gardenAccess garden.Access, vControlPlane virtualenv.ControlPlane, pricingAccess pricing.InstancePricingAccess, targetShootCoord *app.ShootCoordinate) Engine {
	return &engine{
		server: http.Server{
			Addr: ":8080",
		},
		gardenAccess:        gardenAccess,
		virtualControlPlane: vControlPlane,
		pricingAccess:       pricingAccess,
		targetShootCoord:    targetShootCoord,
		recommenderFactory:  factory.New(gardenAccess, vControlPlane, pricingAccess),
	}
}

func (e *engine) Run() {
	e.server.Handler = e.routes()
	if err := e.server.ListenAndServe(); err != nil {
		app.ExitAppWithError(1, fmt.Errorf("error starting scenario http server: %w", err))
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
	return e.recommenderFactory
}

func (e *engine) TargetShootCoordinate() app.ShootCoordinate {
	return *e.targetShootCoord
}

func (e *engine) routes() *http.ServeMux {
	mux := http.NewServeMux()
	h := NewSimulationHandler(e)
	mux.HandleFunc("POST /simulation/", h.run)
	return mux
}
