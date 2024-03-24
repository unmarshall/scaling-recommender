package scaler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"unmarshall/scaling-recommender/internal/garden"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler/scaledown"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

var (
	ErrScalingAlgoVariantNotRegistered = errors.New("requested scaling algo variant is not registered")
)

type StrategyWeights struct {
	LeastWaste float64
	LeastCost  float64
}

type AlgoVariant string

const (
	VanillaScaleUpAlgo               AlgoVariant = "vanilla-scale-up"
	MultiDimensionScoringScaleUpAlgo AlgoVariant = "multi-dimensional-scoring-scale-up"
	DescendingCostScaleDownAlgo      AlgoVariant = "descending-cost-scale-down"
)

type LogWriterFlusher interface {
	io.Writer
	http.Flusher
}

type Result struct {
	Val Recommendation
	Err error
}

type Recommendation struct {
	ScaleUp   map[string]int
	ScaleDown []string
}

type Factory interface {
	GetRecommender(variant AlgoVariant) (Recommender, bool)
}

type Recommender interface {
	Run(ctx context.Context, w LogWriterFlusher) error
}

type factory struct {
	algos map[AlgoVariant]Recommender
}

func NewFactory(vcp virtualenv.ControlPlane, ga garden.Access, pricingAccess pricing.InstancePricingAccess) Factory {
	algos := make(map[AlgoVariant]Recommender)
	// Register all scaling algorithms
	algos[DescendingCostScaleDownAlgo] = scaledown.NewDescendingCostRecommender(vcp.NodeControl(), vcp.PodControl(), pricingAccess)
	return &factory{
		algos: algos,
	}
}

func (f *factory) GetRecommender(variant AlgoVariant) (Recommender, bool) {
	recommender, found := f.algos[variant]
	return recommender, found
}
