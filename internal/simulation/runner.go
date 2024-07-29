package simulation

import (
	"context"
	kvcl "github.com/unmarshall/kvcl/pkg/control"
	"log/slog"
	"net/http"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/api"

	kvclapi "github.com/unmarshall/kvcl/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
)

type Engine interface {
	Start(ctx context.Context) error
	Shutdown()
	VirtualControlPlane() kvclapi.ControlPlane
	PricingAccess() pricing.InstancePricingAccess
	RecommenderFactory() scaler.RecommenderFactory
	ScoringStrategy() string
	TargetClient() client.Client
}

type engine struct {
	server             http.Server
	virtualCluster     kvclapi.ControlPlane
	pricingAccess      pricing.InstancePricingAccess
	recommenderFactory scaler.RecommenderFactory
	appConfig          api.AppConfig
	logger             *slog.Logger
	targetClient       client.Client
}

func NewExecutorEngine(appConfig api.AppConfig, logger *slog.Logger) Engine {
	return &engine{
		server: http.Server{
			Addr: ":8080",
		},
		logger:    logger,
		appConfig: appConfig,
	}
}

func (e *engine) Start(ctx context.Context) error {
	e.startEmbeddedVirtualCluster(ctx)
	if err := e.initializePricingAccess(); err != nil {
		return err
	}
	e.createTargetClient()
	return e.startHTTPServer()
}

func (e *engine) startEmbeddedVirtualCluster(ctx context.Context) {
	vCluster := kvcl.NewControlPlane(e.appConfig.BinaryAssetsPath, "")
	if err := vCluster.Start(ctx); err != nil {
		slog.Error("failed to start virtual cluster", "error", err)
		os.Exit(1)
	}
	e.virtualCluster = vCluster
	slog.Info("virtual cluster started successfully")
}

func (e *engine) initializePricingAccess() error {
	e.logger.Info("Initializing instance pricing access...")
	pricingAccess, err := pricing.NewInstancePricingAccess(e.appConfig.Provider)
	if err != nil {
		return err
	}
	e.pricingAccess = pricingAccess
	return nil
}

func (e *engine) createTargetClient() {

}

func (e *engine) startHTTPServer() error {
	e.server.Handler = e.routes()
	if err := e.server.ListenAndServe(); err != nil {
		return err
	}
	return nil
}

func (e *engine) Shutdown() {
	e.logger.Info("shutting down virtual cluster...")
	if err := e.virtualCluster.Stop(); err != nil {
		e.logger.Error("failed to stop virtual cluster", "error", err)
	}
	if err := e.server.Shutdown(context.Background()); err != nil {
		slog.Error("error shutting down scenario http server", "error", err)
	}
}

func (e *engine) VirtualControlPlane() kvclapi.ControlPlane {
	return e.virtualCluster
}

func (e *engine) PricingAccess() pricing.InstancePricingAccess {
	return e.pricingAccess
}

func (e *engine) RecommenderFactory() scaler.RecommenderFactory {
	return e.recommenderFactory
}

func (e *engine) ScoringStrategy() string {
	return e.appConfig.ScoringStrategy
}

func (e *engine) TargetClient() client.Client {
	return e.targetClient
}

func (e *engine) routes() *http.ServeMux {
	mux := http.NewServeMux()
	h := NewSimulationHandler(e)
	mux.HandleFunc("POST /recommend/", h.run)
	return mux
}
