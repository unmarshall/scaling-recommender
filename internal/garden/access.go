package garden

import (
	"context"
	"log/slog"

	authenticationv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/util"
)

const defaultAdminKubeConfigExpirationSeconds = 86400

type Access interface {
	GetShootAccess(ctx context.Context, shootCoord common.ShootCoordinate) (ShootAccess, error)
	GetShoot(ctx context.Context, shootCoord common.ShootCoordinate) (*gardencorev1beta1.Shoot, error)
	SyncReferenceNodes(ctx context.Context, shootCoord common.ShootCoordinate) error
	GetReferenceNode(instanceType string) (*corev1.Node, bool)
}

func NewAccess(garden string) (Access, error) {
	gardenClient, err := createVirtualGardenClient(garden)
	if err != nil {
		return nil, err
	}
	return &access{
		garden:        garden,
		vGardenClient: gardenClient,
		shootAccesses: make(map[common.ShootCoordinate]ShootAccess),
	}, nil
}

type access struct {
	garden         string
	vGardenClient  client.Client
	shootAccesses  map[common.ShootCoordinate]ShootAccess
	referenceNodes []corev1.Node
}

func (a *access) GetShootAccess(ctx context.Context, shootCoord common.ShootCoordinate) (ShootAccess, error) {
	if sa, found := a.findShootAccess(shootCoord); found {
		return sa, nil
	}
	slog.Info("creating shoot access", "shoot", shootCoord)
	return a.createShootAccess(ctx, shootCoord)
}

func (a *access) GetShoot(ctx context.Context, shootCoord common.ShootCoordinate) (*gardencorev1beta1.Shoot, error) {
	shootNs := shootCoord.GetNamespace()
	shoot := &gardencorev1beta1.Shoot{}
	if err := a.vGardenClient.Get(ctx, client.ObjectKey{Name: shootCoord.Name, Namespace: shootNs}, shoot); err != nil {
		return nil, err
	}
	return shoot, nil
}

func (a *access) SyncReferenceNodes(ctx context.Context, shootCoord common.ShootCoordinate) error {
	sa, err := a.GetShootAccess(ctx, shootCoord)
	if err != nil {
		return err
	}
	nodes, err := sa.ListNodes(ctx, nil)
	if err != nil {
		return err
	}
	a.referenceNodes = nodes
	return nil
}

func (a *access) GetReferenceNode(instanceType string) (*corev1.Node, bool) {
	for _, node := range a.referenceNodes {
		if util.GetNodeInstanceType(node) == instanceType {
			return &node, true
		}
	}
	return nil, false
}

func (a *access) createShootAccess(ctx context.Context, shootCoord common.ShootCoordinate) (ShootAccess, error) {
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

func (a *access) findShootAccess(shootCoord common.ShootCoordinate) (ShootAccess, bool) {
	for coord, sa := range a.shootAccesses {
		if coord.Name == shootCoord.Name && coord.Project == shootCoord.Project {
			// check if the kubeconfig is still valid
			if !sa.HasExpired() {
				return sa, true
			} else {
				slog.Warn("Admin kubeconfig for shoot has expired. ShootAccess should be recreated", "shoot", shootCoord)
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
