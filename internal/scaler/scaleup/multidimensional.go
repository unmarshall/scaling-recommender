package scaleup

import (
	"context"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

type recommender struct {
	nc     virtualenv.NodeControl
	pc     virtualenv.PodControl
	ec     virtualenv.EventControl
	pa     pricing.InstancePricingAccess
	state  simulationState
	logger slog.Logger
}

type simulationState struct {
	originalPods    map[string]corev1.Pod
	existingNodes   []corev1.Node
	unscheduledPods []corev1.Pod
	scheduledPods   []corev1.Pod
	// eligibleNodePools holds the available node capacity per node pool.
	eligibleNodePools map[string]api.NodePool
}

func NewRecommender(vcp virtualenv.ControlPlane, pa pricing.InstancePricingAccess, logger slog.Logger) scaler.Recommender {
	return &recommender{
		nc:     vcp.NodeControl(),
		pc:     vcp.PodControl(),
		ec:     vcp.EventControl(),
		pa:     pa,
		logger: logger,
	}
}

func (r recommender) Run(ctx context.Context, simReq api.SimulationRequest) scaler.Result {
	return scaler.Result{}
}
