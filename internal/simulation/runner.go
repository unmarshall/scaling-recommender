package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"unmarshall/scaling-recommender/internal/app"
	"unmarshall/scaling-recommender/internal/scaler/factory"

	kvclapi "github.com/unmarshall/kvcl/api"
	"unmarshall/scaling-recommender/internal/garden"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
)

type Engine interface {
	Run()
	Shutdown()
	GardenAccess() garden.Access
	VirtualControlPlane() kvclapi.ControlPlane
	PricingAccess() pricing.InstancePricingAccess
	RecommenderFactory() scaler.RecommenderFactory
	TargetShootCoordinate() app.ShootCoordinate
	ScoringStrategy() string
}

type engine struct {
	server              http.Server
	gardenAccess        garden.Access
	virtualControlPlane kvclapi.ControlPlane
	pricingAccess       pricing.InstancePricingAccess
	targetShootCoord    *app.ShootCoordinate
	recommenderFactory  scaler.RecommenderFactory
	scoringStrategy     string
}

func NewExecutor(gardenAccess garden.Access, vControlPlane kvclapi.ControlPlane, pricingAccess pricing.InstancePricingAccess, targetShootCoord *app.ShootCoordinate, logger *slog.Logger, scoringStrategy string) Engine {
	return &engine{
		server: http.Server{
			Addr: ":8080",
		},
		gardenAccess:        gardenAccess,
		virtualControlPlane: vControlPlane,
		pricingAccess:       pricingAccess,
		targetShootCoord:    targetShootCoord,
		recommenderFactory:  factory.New(gardenAccess, vControlPlane, logger),
		scoringStrategy:     scoringStrategy,
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

func (e *engine) VirtualControlPlane() kvclapi.ControlPlane {
	return e.virtualControlPlane
}

func (e *engine) PricingAccess() pricing.InstancePricingAccess {
	return e.pricingAccess
}

func (e *engine) RecommenderFactory() scaler.RecommenderFactory {
	return e.recommenderFactory
}

func (e *engine) TargetShootCoordinate() app.ShootCoordinate {
	return *e.targetShootCoord
}

func (e *engine) ScoringStrategy() string {
	return e.scoringStrategy
}

func (e *engine) routes() *http.ServeMux {
	mux := http.NewServeMux()
	h := NewSimulationHandler(e)
	mux.HandleFunc("POST /simulation/", h.run)
	return mux
}
