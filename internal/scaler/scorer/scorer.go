package scorer

import (
	"fmt"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/scaler/scorer/costcpumemwastage"
	"unmarshall/scaling-recommender/internal/scaler/scorer/costonly"
)

func NewScorer(scoringStrategy string, pa pricing.InstancePricingAccess, nodePools []api.NodePool) (scaler.Scorer, error) {
	switch scoringStrategy {
	case string(CostCpuMemWastageStrategy):
		return costcpumemwastage.NewScorer(pa, nodePools), nil
	case string(CostOnlyStrategy):
		return costonly.NewScorer(pa, nodePools), nil
	default:
		return nil, fmt.Errorf("unknown scoring strategy: %s", scoringStrategy)
	}
}
