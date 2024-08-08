package scaler

import (
	"context"
	"io"
	"k8s.io/apimachinery/pkg/util/sets"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/api"
)

type AlgoVariant string

const (
	DefaultScaleUpAlgo AlgoVariant = "default-scale-up"
)

// ScoringStrategy defines the strategy used to score nodes.
type ScoringStrategy string

const (
	// CostOnlyStrategy is a scoring strategy that scores nodes based on cost only.
	CostOnlyStrategy ScoringStrategy = "cost-only"
)

var scoringStrategies = sets.New(string(CostOnlyStrategy))

// IsScoringStrategySupported checks if the passed in scoring strategy is supported.
func IsScoringStrategySupported(strategy string) bool {
	return scoringStrategies.Has(strategy)
}

type ScorerFactory interface {
	GetScorer(scoringStrategy ScoringStrategy) (Scorer, error)
}

type Scorer interface {
	Compute(scaledNode *corev1.Node, candidatePods []corev1.Pod) float64
}

type LogWriterFlusher interface {
	io.Writer
	http.Flusher
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
