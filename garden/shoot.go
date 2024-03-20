package garden

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/util"
)

// ShootCoordinates represents the coordinates of a shoot cluster. It can be used to represent both the shoot and seed.
type ShootCoordinates struct {
	Project string
	Shoot   string
}

type ShootAccess interface {
	// HasExpired returns true if the admin kube config using which the client is created has expired thus expiring the shoot access
	HasExpired() bool
	// ListNodes will get all nodes and apply the given filters to the nodes in conjunction. If no filters are given, all nodes are returned.
	ListNodes(ctx context.Context, filter ...api.NodeFilter) ([]corev1.Node, error)
	// ListPods will get all pods and apply the given filters to the pods in conjunction. If no filters are given, all pods are returned.
	ListPods(ctx context.Context, filter ...api.PodFilter) ([]corev1.Pod, error)
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

func (s *shootAccess) ListNodes(ctx context.Context, filters ...api.NodeFilter) ([]corev1.Node, error) {
	return util.ListNodes(ctx, s.client, filters...)
}

func (s *shootAccess) ListPods(ctx context.Context, filters ...api.PodFilter) ([]corev1.Pod, error) {
	return util.ListPods(ctx, s.client, filters...)
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
