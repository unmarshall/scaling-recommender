package scorer

import (
	"fmt"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/scaler/scorer/costcpumemwastage"
	"unmarshall/scaling-recommender/internal/scaler/scorer/purecost"
)

func NewScorer(scoringStrategy string, pa pricing.InstancePricingAccess, nodePools []api.NodePool) (scaler.Scorer, error) {
	switch scoringStrategy {
	case string(CostCpuMemWastageStrategy):
		return costcpumemwastage.NewScorer(pa, nodePools), nil
	case string(PureCostStrategy):
		return purecost.NewScorer(pa, nodePools), nil
	default:
		return nil, fmt.Errorf("unknown scoring strategy: %s", scoringStrategy)
	}
}
