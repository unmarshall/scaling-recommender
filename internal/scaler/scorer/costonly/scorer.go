package costonly

import (
	corev1 "k8s.io/api/core/v1"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
)

type _scorer struct {
	instanceTypeCost map[string]float64
}

func NewScorer(pa pricing.InstancePricingAccess) scaler.Scorer {
	//instanceTypeCosts := pa.GetCostForInstanceTypes()
	//return &_scorer{
	//	instanceTypeCost: instanceTypeCosts,
	//}
	panic("not implemented")
}

func (s *_scorer) Compute(scaledNode *corev1.Node, _ []corev1.Pod) float64 {
	//nodeScore := s.instanceTypeCost[util.GetInstanceType(scaledNode.Labels)]
	//return scaler.NodeScore{
	//	CumulativeScore: nodeScore,
	//}
	panic("not implemented")
}
