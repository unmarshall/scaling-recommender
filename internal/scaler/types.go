package scaler

import (
	"context"
	"io"
	"net/http"
	"unmarshall/scaling-recommender/internal/scaler/scorer"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/api"
)

type AlgoVariant string

const (
	MultiDimensionScoringScaleUpAlgo AlgoVariant = "multi-dimensional-scoring-scale-up"
	DescendingCostScaleDownAlgo      AlgoVariant = "descending-cost-scale-down"
)

type NodeScore struct {
	MemWasteRatio    float64
	CpuWasteRatio    float64
	UnscheduledRatio float64
	CostRatio        float64
	CumulativeScore  float64
}

type Scorer interface {
	Compute(scaledNode *corev1.Node, candidatePods []corev1.Pod) NodeScore
	GetScoringStrategy() scorer.ScoringStrategy
}

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
	Run(ctx context.Context, scorer Scorer, simReq api.SimulationRequest) Result
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
