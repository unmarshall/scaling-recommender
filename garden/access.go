package garden

import (
	"context"
	"fmt"
	"log/slog"

	authenticationv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultAdminKubeConfigExpirationSeconds = 86400

type Access interface {
	GetShootAccess(ctx context.Context, shootCoord ShootCoordinates) (ShootAccess, error)
	GetShoot(ctx context.Context, shootCoord ShootCoordinates) (*gardencorev1beta1.Shoot, error)
}

func NewAccess(garden string) (Access, error) {
	gardenClient, err := createVirtualGardenClient(garden)
	if err != nil {
		return nil, err
	}
	return &access{
		garden:        garden,
		vGardenClient: gardenClient,
		shootAccesses: make(map[ShootCoordinates]ShootAccess),
	}, nil
}

type access struct {
	garden        string
	vGardenClient client.Client
	shootAccesses map[ShootCoordinates]ShootAccess
}

func (a *access) GetShootAccess(ctx context.Context, shootCoord ShootCoordinates) (ShootAccess, error) {
	if sa, found := a.findShootAccess(shootCoord); found {
		return sa, nil
	}
	return a.createShootAccess(ctx, shootCoord)
}

func (a *access) GetShoot(ctx context.Context, shootCoord ShootCoordinates) (*gardencorev1beta1.Shoot, error) {
	shootNs := fmt.Sprintf("garden-%s", shootCoord.Project)
	shoot := &gardencorev1beta1.Shoot{}
	if err := a.vGardenClient.Get(ctx, client.ObjectKey{Name: shootCoord.Shoot, Namespace: shootNs}, shoot); err != nil {
		return nil, err
	}
	return shoot, nil
}

func (a *access) createShootAccess(ctx context.Context, shootCoord ShootCoordinates) (ShootAccess, error) {
	shoot, err := a.GetShoot(ctx, shootCoord)
	if err != nil {
		return nil, err
	}
	adminKubeconfig, err := a.createShootAdminKubeConfig(ctx, shoot)
	if err != nil {
		return nil, err
	}

	sa, err := NewShootAccess(a.garden, shootCoord, adminKubeconfig)
	if err != nil {
		return nil, err
	}
	a.shootAccesses[shootCoord] = sa
	return sa, nil
}

func (a *access) findShootAccess(shootCoord ShootCoordinates) (ShootAccess, bool) {
	for coord, sa := range a.shootAccesses {
		if coord.Shoot == shootCoord.Shoot && coord.Project == shootCoord.Project {
			// check if the kubeconfig is still valid
			if !sa.HasExpired() {
				return sa, true
			} else {
				slog.Warn("Admin kubeconfig for shoot has expired. Recreating shoot access", "shoot", shootCoord)
			}
		}
	}
	return nil, false
}

func (a *access) createShootAdminKubeConfig(ctx context.Context, shoot *gardencorev1beta1.Shoot) ([]byte, error) {
	adminKubeconfigRequest := authenticationv1alpha1.AdminKubeconfigRequest{
		Spec: authenticationv1alpha1.AdminKubeconfigRequestSpec{
			ExpirationSeconds: pointer.Int64(defaultAdminKubeConfigExpirationSeconds),
		},
	}
	if err := a.vGardenClient.SubResource("adminkubeconfig").Create(ctx, shoot, &adminKubeconfigRequest); err != nil {
		return nil, err
	}
	return adminKubeconfigRequest.Status.Kubeconfig, nil
}

func createVirtualGardenClient(name string) (client.Client, error) {
	config, err := loadGardenConfig()
	if err != nil {
		return nil, err
	}
	gardenConfig, err := config.getVirtualGardenConfig(name)
	if err != nil {
		return nil, err
	}
	loader := &clientcmd.ClientConfigLoadingRules{ExplicitPath: gardenConfig.KubeConfigPath}
	overrides := &clientcmd.ConfigOverrides{}
	if gardenConfig.Context != "" {
		overrides.CurrentContext = gardenConfig.Context
	}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	return client.New(restCfg, client.Options{})
}
