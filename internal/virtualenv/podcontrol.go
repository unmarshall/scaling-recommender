package virtualenv

import (
	"context"
	"errors"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/util"
)

type PodControl interface {
	// ListPods will get all pods and apply the given filters to the pods in conjunction. If no filters are given, all pods are returned.
	ListPods(ctx context.Context, filters ...common.PodFilter) ([]corev1.Pod, error)
	// CreatePodsAsUnscheduled creates new unscheduled pods in the in-memory controlPlane from the given schedulerName and pod specs.
	CreatePodsAsUnscheduled(ctx context.Context, schedulerName string, pods ...corev1.Pod) error
	// CreatePods creates new pods in the in-memory controlPlane.
	CreatePods(ctx context.Context, pods ...corev1.Pod) error
	// DeletePods deletes the given pods from the in-memory controlPlane.
	DeletePods(ctx context.Context, pods ...corev1.Pod) error
	// DeleteAllPods deletes all pods from the in-memory controlPlane.
	DeleteAllPods(ctx context.Context) error
}

type podControl struct {
	client client.Client
}

func NewPodControl(cl client.Client) PodControl {
	return &podControl{
		client: cl,
	}
}

func (p podControl) ListPods(ctx context.Context, filters ...common.PodFilter) ([]corev1.Pod, error) {
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

func (p podControl) CreatePods(ctx context.Context, pods ...corev1.Pod) error {
	var errs error
	for _, pod := range pods {
		clone := pod.DeepCopy()
		clone.ObjectMeta.UID = ""
		clone.ObjectMeta.ResourceVersion = ""
		clone.ObjectMeta.CreationTimestamp = metav1.Time{}
		errors.Join(errs, p.client.Create(ctx, clone))
	}
	return errs
}

func (p podControl) DeletePods(ctx context.Context, pods ...corev1.Pod) error {
	var errs error
	podsFailedDeletion := make([]string, 0, len(pods))
	for _, pod := range pods {
		if err := p.client.Delete(ctx, &pod); err != nil {
			podsFailedDeletion = append(podsFailedDeletion, pod.Name)
			errors.Join(errs, err)
		}
	}
	if errs != nil {
		slog.Error("failed to delete one or more pods", "pods", podsFailedDeletion, "error", errs)
	}
	return errs
}

func (p podControl) DeleteAllPods(ctx context.Context) error {
	return p.client.DeleteAllOf(ctx, &corev1.Pod{})
}
