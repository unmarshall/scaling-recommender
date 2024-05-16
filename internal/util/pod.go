package util

import (
	"context"
	"slices"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"unmarshall/scaling-recommender/api"

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

func GetPodNames(pods []*corev1.Pod) []string {
	return lo.Map[*corev1.Pod, string](pods, func(pod *corev1.Pod, _ int) string {
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

func ConstructNewPods(pods []corev1.Pod, nodeName, schedulerName string) ([]corev1.Pod, error) {
	var newPods []corev1.Pod
	for _, pod := range pods {
		suffix, err := GenerateRandomString(4)
		if err != nil {
			return nil, err
		}
		newPod := pod.DeepCopy()
		newPod.Spec.NodeName = nodeName
		newPod.Spec.SchedulerName = schedulerName
		if lo.IsEmpty(pod.GenerateName) {
			newPod.Name = pod.Name + "-" + suffix
		} else {
			newPod.Name = ""
		}
		newPods = append(newPods, *newPod)
	}
	return newPods, nil

}

func evaluatePodFilters(pod *corev1.Pod, filters []common.PodFilter) bool {
	for _, f := range filters {
		if ok := f(pod); !ok {
			return false
		}
	}
	return true
}

func ConstructPodsFromPodInfos(podInfos []api.PodInfo, sortOrder string) []*corev1.Pod {
	pods := make([]*corev1.Pod, 0, len(podInfos))
	for _, podInfo := range podInfos {
		podBuilder := NewPodBuilder().
			NamePrefix(podInfo.NamePrefix).
			SchedulerName(common.BinPackingSchedulerName).
			Labels(podInfo.Labels).
			ResourceRequests(podInfo.Requests).
			TopologySpreadConstraints(podInfo.TopologySpreadConstraints).
			Tolerations(podInfo.Tolerations).
			Count(podInfo.Count)
		if podInfo.ScheduledOn != nil {
			podBuilder.ScheduledOn(podInfo.ScheduledOn.Name)
		}
		pods = append(pods, podBuilder.Build()...)
	}
	sortPods(pods, sortOrder)
	return pods
}

func SplitScheduledAndUnscheduledPods(pods []*corev1.Pod) (unscheduledPods []*corev1.Pod, scheduledPods []*corev1.Pod) {
	for _, pod := range pods {
		if isUnscheduled(pod) {
			unscheduledPods = append(unscheduledPods, pod)
		} else {
			scheduledPods = append(scheduledPods, pod)
		}
	}
	return
}

func isUnscheduled(pod *corev1.Pod) bool {
	return !isScheduled(pod) &&
		!isPreempting(pod) &&
		!isOwnedByDaemonSet(pod)
}

func isScheduled(pod *corev1.Pod) bool {
	return pod.Spec.NodeName != ""
}

func isPreempting(pod *corev1.Pod) bool {
	return pod.Status.NominatedNodeName != ""
}

func isOwnedByDaemonSet(pod *corev1.Pod) bool {
	return isOwnedBy(pod, []schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "DaemonSet"},
	})
}

func isOwnedBy(pod *corev1.Pod, gvks []schema.GroupVersionKind) bool {
	for _, ignoredOwner := range gvks {
		for _, owner := range pod.ObjectMeta.OwnerReferences {
			if owner.APIVersion == ignoredOwner.GroupVersion().String() && owner.Kind == ignoredOwner.Kind {
				return true
			}
		}
	}
	return false
}

func sortPods(pods []*corev1.Pod, sortOrder string) {
	if sortOrder == "desc" {
		slices.SortFunc(pods, sortDescendingFn())
	} else {
		slices.SortFunc(pods, sortAscendingFn())
	}
}

func sortAscendingFn() func(*corev1.Pod, *corev1.Pod) int {
	return func(podA, podB *corev1.Pod) int {
		return podA.Spec.Containers[0].Resources.Requests.Memory().Cmp(*podB.Spec.Containers[0].Resources.Requests.Memory())
	}
}

func sortDescendingFn() func(*corev1.Pod, *corev1.Pod) int {
	return func(podA, podB *corev1.Pod) int {
		return -podA.Spec.Containers[0].Resources.Requests.Memory().Cmp(*podB.Spec.Containers[0].Resources.Requests.Memory())
	}
}
