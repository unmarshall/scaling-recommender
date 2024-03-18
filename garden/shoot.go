package garden

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ShootCoordinates struct {
	Project string
	Shoot   string
}

type ShootAccess interface {
	// GetNodes returns the current nodes of the shoot.
	GetNodes(ctx context.Context) ([]corev1.Node, error)
	// HasExpired returns true if the admin kube config using which the client is created has expired thus expiring the shoot access
	HasExpired() bool
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

func (s *shootAccess) GetNodes(ctx context.Context) ([]corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := s.client.List(ctx, nodes)
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

func (s *shootAccess) HasExpired() bool {
	return time.Now().After(s.createdAt.Add(defaultAdminKubeConfigExpirationSeconds * time.Second))
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
