package garden

import (
	"time"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ShootCoordinates struct {
	Project string
	Shoot   string
}

type ShootAccess interface {
	SyncNodes() error
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

func (s *shootAccess) SyncNodes() error {
	return nil
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
