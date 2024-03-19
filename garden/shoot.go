package garden

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ShootCoordinates represents the coordinates of a shoot cluster. It can be used to represent both the shoot and seed.
type ShootCoordinates struct {
	Project string
	Shoot   string
}

// PodFilter is a predicate that takes in a Pod and returns the predicate result as a boolean.
type PodFilter func(pod *corev1.Pod) bool

type ShootAccess interface {
	// HasExpired returns true if the admin kube config using which the client is created has expired thus expiring the shoot access
	HasExpired() bool
	// GetNodes returns the current nodes of the shoot.
	GetNodes(ctx context.Context) ([]corev1.Node, error)
	// GetPods will get all pods and apply the given filters to the pods in conjunction. If no filters are given, all pods are returned.
	GetPods(ctx context.Context, filter ...PodFilter) ([]corev1.Pod, error)
}

type shootAccess struct {
	garden     string
	shootCoord ShootCoordinates
	client     client.Client
	createdAt  time.Time
}

func NewShootAccess(garden string, shootCoord ShootCoordinates, kubeConfig []byte) (ShootAccess, error) {
	cl, err := createShootClient(kubeConfig)
	if err != nil {
		return nil, err
	}
	return &shootAccess{
		garden:     garden,
		shootCoord: shootCoord,
		client:     cl,
		createdAt:  time.Now(),
	}, nil
}

func (s *shootAccess) HasExpired() bool {
	return time.Now().After(s.createdAt.Add(defaultAdminKubeConfigExpirationSeconds * time.Second))
}

func (s *shootAccess) GetNodes(ctx context.Context) ([]corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := s.client.List(ctx, nodes)
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

func (s *shootAccess) GetPods(ctx context.Context, filters ...PodFilter) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}
	err := s.client.List(ctx, pods)
	if err != nil {
		return nil, err
	}
	if filters == nil {
		return pods.Items, nil
	}
	filteredPods := make([]corev1.Pod, 0, len(pods.Items))
	for _, p := range pods.Items {
		if ok := evaluateFilters(&p, filters); ok {
			filteredPods = append(filteredPods, p)
		}
	}
	return filteredPods, nil
}

func evaluateFilters(pod *corev1.Pod, filters []PodFilter) bool {
	for _, f := range filters {
		if ok := f(pod); !ok {
			return false
		}
	}
	return true
}

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

func createShootClient(kubeConfigBytes []byte) (client.Client, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
	if err != nil {
		return nil, err
	}
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return client.New(restConfig, client.Options{})
}
