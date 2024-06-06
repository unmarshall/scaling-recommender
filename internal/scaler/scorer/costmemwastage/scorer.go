package costmemwastage

import (
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/util"
)

type _scorer struct {
	instanceTypeCostRatio map[string]float64
}

func NewScorer(pa pricing.InstancePricingAccess, nodePools []api.NodePool) scaler.Scorer {
	instanceTypeCostRatios := pa.ComputeCostRatiosForInstanceTypes(nodePools)
	return &_scorer{
		instanceTypeCostRatio: instanceTypeCostRatios,
	}
}

func (s *_scorer) Compute(scaledNode *corev1.Node, candidatePods []corev1.Pod) scaler.NodeScore {
	costRatio := s.instanceTypeCostRatio[util.GetInstanceType(scaledNode.Labels)]
	wasteRatio := computeWasteRatio(scaledNode, candidatePods)
	unscheduledRatio := computeUnscheduledRatio(candidatePods)
	cumulativeScore := wasteRatio + unscheduledRatio*costRatio
	return scaler.NodeScore{
		MemWasteRatio:    wasteRatio,
		UnscheduledRatio: unscheduledRatio,
		CostRatio:        costRatio,
		CumulativeScore:  cumulativeScore,
	}
}

func computeWasteRatio(node *corev1.Node, candidatePods []corev1.Pod) float64 {
	var (
		targetNodeAssignedPods []corev1.Pod
		totalMemoryConsumed    int64
	)
	for _, pod := range candidatePods {
		if pod.Spec.NodeName == node.Name {
			targetNodeAssignedPods = append(targetNodeAssignedPods, pod)
			for _, container := range pod.Spec.Containers {
				containerMemReq, ok := container.Resources.Requests[corev1.ResourceMemory]
				if ok {
					totalMemoryConsumed += containerMemReq.MilliValue()
				}
			}
			slog.Info("NodPodAssignment: ", "pod", pod.Name, "node", pod.Spec.NodeName, "memory", pod.Spec.Containers[0].Resources.Requests.Memory().MilliValue())
		}
	}
	totalMemoryCapacity := node.Status.Capacity.Memory().MilliValue()
	return float64(totalMemoryCapacity-totalMemoryConsumed) / float64(totalMemoryCapacity)
}

func computeUnscheduledRatio(candidatePods []corev1.Pod) float64 {
	var totalAssignedPods int
	for _, pod := range candidatePods {
		if pod.Spec.NodeName != "" {
			totalAssignedPods++
		}
	}
	return float64(len(candidatePods)-totalAssignedPods) / float64(len(candidatePods))
}
