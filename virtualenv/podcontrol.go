package virtualenv

import (
	"context"
	"errors"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/util"
)

type PodControl interface {
	// ListPods will get all pods and apply the given filters to the pods in conjunction. If no filters are given, all pods are returned.
	ListPods(ctx context.Context, filters ...api.PodFilter) ([]corev1.Pod, error)
	// CreatePodsAsUnscheduled creates new unscheduled pods in the in-memory controlPlane from the given schedulerName and pod specs.
	CreatePodsAsUnscheduled(ctx context.Context, schedulerName string, pods ...corev1.Pod) error
}

type podControl struct {
	client client.Client
}

func NewPodControl(cl client.Client) PodControl {
	return &podControl{
		client: cl,
	}
}

func (p podControl) ListPods(ctx context.Context, filters ...api.PodFilter) ([]corev1.Pod, error) {
	return util.ListPods(ctx, p.client, filters...)
}

func (p podControl) CreatePodsAsUnscheduled(ctx context.Context, schedulerName string, pods ...corev1.Pod) error {
	var errs error
	for _, pod := range pods {
		podObjMeta := metav1.ObjectMeta{
			Namespace:       pod.Namespace,
			OwnerReferences: pod.OwnerReferences,
			Labels:          pod.Labels,
			Annotations:     pod.Annotations,
		}
		if pod.GenerateName != "" {
			podObjMeta.GenerateName = pod.GenerateName
		} else {
			podObjMeta.Name = pod.Name
		}
		dupPod := &corev1.Pod{
			ObjectMeta: podObjMeta,
			Spec:       pod.Spec,
		}
		dupPod.Spec.NodeName = ""
		dupPod.Spec.SchedulerName = schedulerName
		dupPod.Spec.TerminationGracePeriodSeconds = ptr.To(int64(0))
		if err := p.client.Create(ctx, dupPod); err != nil {
			slog.Error("failed to create pod in virtual controlPlane", "pod", client.ObjectKeyFromObject(dupPod), "error", err)
			errors.Join(errs, err)
		}
	}
	return errs
}
