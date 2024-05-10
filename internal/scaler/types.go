package scaler

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"unmarshall/scaling-recommender/internal/garden"
	"unmarshall/scaling-recommender/internal/scaler/scaleup"

	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

type AlgoVariant string

const (
	MultiDimensionScoringScaleUpAlgo AlgoVariant = "multi-dimensional-scoring-scale-up"
	DescendingCostScaleDownAlgo      AlgoVariant = "descending-cost-scale-down"
)

type LogWriterFlusher interface {
	io.Writer
	http.Flusher
}

type Result struct {
	Ok  api.Recommendation
	Err error
}

func (r Result) IsError() bool {
	return r.Err != nil
}

func ErrorResult(err error) Result {
	return Result{Err: err}
}

func OkResult(recommendation api.Recommendation) Result {
	return Result{Ok: recommendation}
}

func OkScaleUpResult(recommendations []api.ScaleUpRecommendation) Result {
	return Result{Ok: api.Recommendation{ScaleUp: recommendations}}
}

func NewScaleDownRecommendation(scaleDown []string) api.Recommendation {
	return api.Recommendation{ScaleDown: scaleDown}
}

type Factory interface {
	GetRecommender(variant AlgoVariant) Recommender
}

type Recommender interface {
	Run(ctx context.Context, simReq api.SimulationRequest, logger slog.Logger) Result
}

type factory struct {
	algos map[AlgoVariant]Recommender
}

func NewFactory(ga garden.Access, vcp virtualenv.ControlPlane, pricingAccess pricing.InstancePricingAccess) Factory {
	algos := make(map[AlgoVariant]Recommender)
	// Register all scaling algorithms
	algos[MultiDimensionScoringScaleUpAlgo] = scaleup.NewRecommender(vcp, ga.GetAllReferenceNodes(), pricingAccess)
	//algos[DescendingCostScaleDownAlgo] = scaledown.NewDescendingCostRecommender(vcp.NodeControl(), vcp.PodControl(), vcp.EventControl(), pricingAccess)
	return &factory{
		algos: algos,
	}
}

func (f *factory) GetRecommender(variant AlgoVariant) Recommender {
	return f.algos[variant]
}
