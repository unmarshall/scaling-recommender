package garden

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/app"
	"unmarshall/scaling-recommender/internal/common"
	util2 "unmarshall/scaling-recommender/internal/util"
)

type ShootAccess interface {
	// HasExpired returns true if the admin kube config using which the client is created has expired thus expiring the shoot access
	HasExpired() bool
	// ListNodes will get all nodes and apply the given filters to the nodes in conjunction. If no filters are given, all nodes are returned.
	ListNodes(ctx context.Context, filter ...common.NodeFilter) ([]corev1.Node, error)
	// ListPods will get all pods and apply the given filters to the pods in conjunction. If no filters are given, all pods are returned.
	ListPods(ctx context.Context, namespace string, filter ...common.PodFilter) ([]corev1.Pod, error)

	GetClient() client.Client
}

type shootAccess struct {
	garden     string
	shootCoord app.ShootCoordinate
	client     client.Client
	createdAt  time.Time
}

func NewShootAccess(garden string, shootCoord app.ShootCoordinate, kubeConfig []byte) (ShootAccess, error) {
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

func (s *shootAccess) ListNodes(ctx context.Context, filters ...common.NodeFilter) ([]corev1.Node, error) {
	return util2.ListNodes(ctx, s.client, filters...)
}

func (s *shootAccess) ListPods(ctx context.Context, namespace string, filters ...common.PodFilter) ([]corev1.Pod, error) {
	return util2.ListPods(ctx, s.client, namespace, filters...)
}

func (s *shootAccess) GetClient() client.Client {
	return s.client
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
