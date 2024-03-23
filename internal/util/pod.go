package util

import (
	"context"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/common"
)

// NotYetScheduledPod is a PodFilter that returns true if the pod is not yet scheduled.
func NotYetScheduledPod(pod *corev1.Pod) bool {
	return pod.Spec.NodeName == ""
}

// PodSchedulingFailed is a PodFilter that returns true if the pod scheduling has failed.
func PodSchedulingFailed(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Reason == corev1.PodReasonUnschedulable {
			return true
		}
	}
	return false
}

func GetPodNames(pods []corev1.Pod) []string {
	return lo.Map[corev1.Pod, string](pods, func(pod corev1.Pod, _ int) string {
		return pod.Name
	})
}

// ListPods will get all pods and apply the given filters to the pods in conjunction. If no filters are given, all pods are returned.
func ListPods(ctx context.Context, cl client.Client, filters ...common.PodFilter) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}
	err := cl.List(ctx, pods)
	if err != nil {
		return nil, err
	}
	if filters == nil {
		return pods.Items, nil
	}
	filteredPods := make([]corev1.Pod, 0, len(pods.Items))
	for _, p := range pods.Items {
		if ok := evaluatePodFilters(&p, filters); ok {
			filteredPods = append(filteredPods, p)
		}
	}
	return filteredPods, nil
}

func evaluatePodFilters(pod *corev1.Pod, filters []common.PodFilter) bool {
	for _, f := range filters {
		if ok := f(pod); !ok {
			return false
		}
	}
	return true
}
