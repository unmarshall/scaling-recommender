package leastcost

import (
	corev1 "k8s.io/api/core/v1"
	"math"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/scaler/scorer"
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

func (s *_scorer) GetScoringStrategy() scorer.ScoringStrategy {
	return scorer.LeastCostStrategy
}

func (s *_scorer) Compute(scaledNode *corev1.Node, candidatePods []corev1.Pod) scaler.NodeScore {
	totalResourceUnitsScheduled := 0.0

	instanceTypeCost := s.instanceTypeCost[util.GetInstanceType(scaledNode.Labels)]

	for _, pod := range candidatePods {
		if pod.Spec.NodeName == scaledNode.Name {
			for _, container := range pod.Spec.Containers {
				containerMemReq, ok := container.Resources.Requests[corev1.ResourceMemory]
				if ok {
					memInGB := containerMemReq.Value() / (1024 * 1024 * 1024) //TODO: verify whether mem is stored in bytes
					totalResourceUnitsScheduled = totalResourceUnitsScheduled + (float64(memInGB) * ResourceUnitsPerMemory)

				}
				containerCPUReq, ok := container.Resources.Requests[corev1.ResourceCPU]
				if ok {
					cpuInCore := containerCPUReq.Value() //TODO: verify whether cpu stored in core always
					totalResourceUnitsScheduled = totalResourceUnitsScheduled + (float64(cpuInCore) * ResourceUnitsPerCPU)
				}
			}
		}
	}

	cumScore := math.Inf(1)
	if totalResourceUnitsScheduled != 0 {
		cumScore = instanceTypeCost / totalResourceUnitsScheduled
	}

	return scaler.NodeScore{
		CumulativeScore: cumScore,
	}
}
