package scaler

import (
	"context"
	"io"
	"log/slog"
	"net/http"

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
