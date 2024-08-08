package scorer

import (
	"fmt"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/scaler/scorer/costonly"
)

type factory struct {
	pa pricing.InstancePricingAccess
}

func NewFactory(pa pricing.InstancePricingAccess) scaler.ScorerFactory {
	return &factory{
		pa: pa,
	}
}

func (f factory) GetScorer(scoringStrategy scaler.ScoringStrategy) (scaler.Scorer, error) {
	switch scoringStrategy {
	case scaler.CostOnlyStrategy:
		return costonly.NewScorer(f.pa), nil
	default:
		return nil, fmt.Errorf("unknown scoring strategy: %s", scoringStrategy)
	}
}
