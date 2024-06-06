package scaler

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/api"
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

func NewScaleDownRecommendation(scaleDown []string) api.Recommendation {
	return api.Recommendation{ScaleDown: scaleDown}
}

type RecommenderFactory interface {
	GetRecommender(variant AlgoVariant) Recommender
}

type Recommender interface {
	Run(ctx context.Context, simReq api.SimulationRequest, logger slog.Logger) Result
}

type OkResult struct {
	Recommendation  api.Recommendation
	UnscheduledPods []client.ObjectKey
}

type Result struct {
	Ok  OkResult
	Err error
}

func (r Result) IsError() bool {
	return r.Err != nil
}

func ErrorResult(err error) Result {
	return Result{Err: err}
}

func OkScaleUpResult(recommendations []api.ScaleUpRecommendation, unscheduledPods []client.ObjectKey) Result {
	return Result{
		Ok: OkResult{
			Recommendation:  api.Recommendation{ScaleUp: recommendations},
			UnscheduledPods: unscheduledPods,
		},
	}
}
