package garden

import (
	"context"
	"log/slog"

	authenticationv1alpha1 "github.com/gardener/gardener/pkg/apis/authentication/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/app"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/util"
)

const defaultAdminKubeConfigExpirationSeconds = 86400

type Access interface {
	GetShootAccess(ctx context.Context, shootCoord app.ShootCoordinate) (ShootAccess, error)
	GetShoot(ctx context.Context, shootCoord app.ShootCoordinate) (*gardencorev1beta1.Shoot, error)
	SyncReferenceNodes(ctx context.Context, shootCoord app.ShootCoordinate) error
	GetReferenceNode(instanceType string) (*corev1.Node, bool)
	GetAllReferenceNodes() []corev1.Node
}

func NewAccess(garden string) (Access, error) {
	gardenClient, err := createVirtualGardenClient(garden)
	if err != nil {
		return nil, err
	}
	return &access{
		garden:        garden,
		vGardenClient: gardenClient,
		shootAccesses: make(map[app.ShootCoordinate]ShootAccess),
	}, nil
}

type access struct {
	garden         string
	vGardenClient  client.Client
	shootAccesses  map[app.ShootCoordinate]ShootAccess
	referenceNodes []corev1.Node
}

func (a *access) GetShootAccess(ctx context.Context, shootCoord app.ShootCoordinate) (ShootAccess, error) {
	if sa, found := a.findShootAccess(shootCoord); found {
		return sa, nil
	}
	slog.Info("creating shoot access", "shoot", shootCoord)
	return a.createShootAccess(ctx, shootCoord)
}

func (a *access) GetShoot(ctx context.Context, shootCoord app.ShootCoordinate) (*gardencorev1beta1.Shoot, error) {
	shootNs := shootCoord.GetNamespace()
	shoot := &gardencorev1beta1.Shoot{}
	if err := a.vGardenClient.Get(ctx, client.ObjectKey{Name: shootCoord.Name, Namespace: shootNs}, shoot); err != nil {
		return nil, err
	}
	return shoot, nil
}

func (a *access) SyncReferenceNodes(ctx context.Context, shootCoord app.ShootCoordinate) error {
	sa, err := a.GetShootAccess(ctx, shootCoord)
	if err != nil {
		return err
	}
	nodes, err := sa.ListNodes(ctx)
	if err != nil {
		return err
	}
	nodesWithRevisedAllocatable, err := reviseNodeAllocatable(ctx, sa, nodes)
	if err != nil {
		return err
	}
	a.referenceNodes = nodesWithRevisedAllocatable
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

func (a *access) GetAllReferenceNodes() []corev1.Node {
	return a.referenceNodes
}

func (a *access) createShootAccess(ctx context.Context, shootCoord app.ShootCoordinate) (ShootAccess, error) {
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

func (a *access) findShootAccess(shootCoord app.ShootCoordinate) (ShootAccess, bool) {
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

// reviseNodeAllocatable will reduce the allocatable for a node by subtracting the total CPU and Memory that is consumed
// by all system components that are deployed in the kube-system namespace of the shoot cluster.
func reviseNodeAllocatable(ctx context.Context, sa ShootAccess, nodes []corev1.Node) ([]corev1.Node, error) {
	maxResourceList, err := getMaxSystemComponentRequestsAcrossNodes(ctx, sa)
	if err != nil {
		return nil, err
	}
	revisedNodes := make([]corev1.Node, 0, len(nodes))
	for _, n := range nodes {
		revisedNode := n.DeepCopy()
		revisedMem := revisedNode.Status.Allocatable.Memory()
		revisedMem.Sub(maxResourceList[corev1.ResourceMemory])
		revisedCPU := revisedNode.Status.Allocatable.Cpu()
		revisedCPU.Sub(maxResourceList[corev1.ResourceCPU])
		revisedNode.Status.Allocatable[corev1.ResourceMemory] = *revisedMem
		revisedNode.Status.Allocatable[corev1.ResourceCPU] = *revisedCPU
		revisedNodes = append(revisedNodes, *revisedNode)
	}
	return revisedNodes, nil
}

func getMaxSystemComponentRequestsAcrossNodes(ctx context.Context, sa ShootAccess) (corev1.ResourceList, error) {
	pods, err := sa.ListPods(ctx, common.KubeSystemNamespace, util.IsSystemPod)
	if err != nil {
		return nil, err
	}
	nodeSystemComponentResourceList := collectSystemComponentResourceRequestsByNode(pods)
	maxResourceList := corev1.ResourceList{}
	for _, r := range nodeSystemComponentResourceList {
		for name, q := range r {
			val, ok := maxResourceList[name]
			if !ok {
				maxResourceList[name] = q
				continue
			}
			if val.Cmp(q) < 0 {
				maxResourceList[name] = q
			}
		}
	}
	return maxResourceList, nil
}

func collectSystemComponentResourceRequestsByNode(pods []corev1.Pod) map[string]corev1.ResourceList {
	podsByNode := lo.GroupBy(pods, func(pod corev1.Pod) string {
		return pod.Spec.NodeName
	})
	nodeResourceRequests := make(map[string]corev1.ResourceList, len(podsByNode))
	for nodeName, nodePods := range podsByNode {
		nodeResourceRequests[nodeName] = sumResourceRequests(nodePods)
	}
	return nodeResourceRequests
}

func sumResourceRequests(pods []corev1.Pod) corev1.ResourceList {
	var totalMemory resource.Quantity
	var totalCPU resource.Quantity
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			totalMemory.Add(util.NilOr(container.Resources.Requests.Memory(), resource.Quantity{}))
			totalCPU.Add(util.NilOr(container.Resources.Requests.Cpu(), resource.Quantity{}))
		}
	}
	return corev1.ResourceList{
		corev1.ResourceMemory: totalMemory,
		corev1.ResourceCPU:    totalCPU,
	}
}
