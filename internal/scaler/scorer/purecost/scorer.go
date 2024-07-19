package purecost

import (
	corev1 "k8s.io/api/core/v1"
	"log/slog"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/util"
)

type _scorer struct {
	instanceTypeCost map[string]float64
}

func NewScorer(pa pricing.InstancePricingAccess, nodePools []api.NodePool) scaler.Scorer {
	instanceTypeCosts := pa.GetCostForInstanceTypes(nodePools)
	return &_scorer{
		instanceTypeCost: instanceTypeCosts,
	}
}

func (s *_scorer) Compute(scaledNode *corev1.Node, _ []corev1.Pod) scaler.NodeScore {
	slog.Info("In Compute method of purecost")
	nodeScore := s.instanceTypeCost[util.GetInstanceType(scaledNode.Labels)]
	return scaler.NodeScore{
		CumulativeScore: nodeScore,
	}
}
