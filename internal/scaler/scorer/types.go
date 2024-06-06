package scorer

import corev1 "k8s.io/api/core/v1"

type NodeScore struct {
	WasteRatio       float64
	UnscheduledRatio float64
	CostRatio        float64
	CumulativeScore  float64
}

type Scorer interface {
	Compute(scaledNode *corev1.Node, candidatePods []corev1.Pod) NodeScore
}
