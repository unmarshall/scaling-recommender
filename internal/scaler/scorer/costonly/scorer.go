package costonly

import (
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/util"

	corev1 "k8s.io/api/core/v1"
)

type _scorer struct {
	pa pricing.InstancePricingAccess
}

func NewScorer(pa pricing.InstancePricingAccess) scaler.Scorer {
	return &_scorer{
		pa: pa,
	}
}

func (s *_scorer) Compute(scaledNode *corev1.Node, scheduledPods []corev1.Pod) float64 {
	instanceCost := s.pa.GetOnDemandPricing(util.GetInstanceType(scaledNode.Labels))
	totalResourceUnitsScheduled := 0.0
	for _, pod := range scheduledPods {
		for _, container := range pod.Spec.Containers {
			containerMemReq, ok := container.Resources.Requests[corev1.ResourceMemory]
			if ok {
				memInGB := containerMemReq.Value() / (1024 * 1024 * 1024) //TODO: verify whether mem is stored in bytes
				totalResourceUnitsScheduled = totalResourceUnitsScheduled + (float64(memInGB) * scaler.MemResourceUnitMultiplier)
			}
			containerCPUReq, ok := container.Resources.Requests[corev1.ResourceCPU]
			if ok {
				cpuInCore := containerCPUReq.Value() //TODO: verify whether cpu stored in core always
				totalResourceUnitsScheduled = totalResourceUnitsScheduled + (float64(cpuInCore) * scaler.CPUResourceUnitMultiplier)
			}
		}
	}
	return totalResourceUnitsScheduled / instanceCost
}
