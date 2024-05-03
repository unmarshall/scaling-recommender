package scaledown

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/garden"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/simulation/web"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

const (
	poolNameLabel           = "worker.gardener.cloud/pool"
	workerGroupNameLabelKey = "worker.gardener.sapcloud.io/group"
	zoneLabelKey            = "topology.kubernetes.io/zone"
	regionLabelKey          = "topology.kubernetes.io/region"
	toRemoveZoneLabelKey    = "failure-domain.beta.kubernetes.io/zone"
	toRemoveRegionLabelKey  = "failure-domain.beta.kubernetes.io/region"
)

type PodRequests struct {
	LargePodCount int
	SmallPodCount int
}

func SetupScaleDownScenario(ctx context.Context,
	vc virtualenv.ControlPlane,
	gc garden.Access,
	w scaler.LogWriterFlusher,
	targetShootCoord common.ShootCoordinate,
	requests PodRequests) error {

	web.Log(w, "Clearing virtual control plane...")
	if err := vc.FactoryReset(ctx); err != nil {
		return fmt.Errorf("failed to reset virtual control plane: %w", err)
	}
	web.Logf(w, "Syncing nodes from shoot cluster %v", targetShootCoord)
	shoot, err := gc.GetShoot(ctx, targetShootCoord)
	if err != nil {
		return fmt.Errorf("failed to get shoot %s: %w", targetShootCoord, err)
	}
	existingNodes, err := getExistingNodes(ctx, gc, targetShootCoord)
	if err != nil {
		return fmt.Errorf("failed to get existing nodes from shoot %s: %w", targetShootCoord, err)
	}
	nps := extractNodePools(shoot, existingNodes)
	for _, np := range nps {
		web.Logf(w, "Creating %d nodes of type: %s in pool %s", np.Current, np.InstanceType, np.Name)
		referenceNode, ok := gc.GetReferenceNode(np.InstanceType)
		if !ok {
			return fmt.Errorf("reference node not found for instance type: %s", np.InstanceType)
		}
		web.Logf(w, "Creating #%d nodes of type: %s in pool %s", np.Max, np.InstanceType, np.Name)
		if err = createLocalNodesOfReferenceNode(ctx, vc.NodeControl(), referenceNode, np, int(np.Max)); err != nil {
			return fmt.Errorf("failed to create nodes in virtual control plane: %w", err)
		}
	}
	return nil
}

func ParseQueryParams(r *http.Request, defaultLarge, defaultSmall int) PodRequests {
	return PodRequests{
		LargePodCount: web.GetIntQueryParam(r, "large", defaultLarge),
		SmallPodCount: web.GetIntQueryParam(r, "small", defaultSmall),
	}
}

func createLocalNodesOfReferenceNode(ctx context.Context, nc virtualenv.NodeControl, referenceNode *corev1.Node, np api.NodePool, count int) error {
	labels := referenceNode.Labels
	labels[workerGroupNameLabelKey] = np.Name
	nodes := make([]corev1.Node, 0, count*len(np.Zones))
	for _, zone := range np.Zones {

	}
	for i := 0; i < count; i++ {
		node := corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf("%s-", np.Name),
				Namespace:    "default",
				Labels:       labels,
			},
			Status: corev1.NodeStatus{
				Allocatable: referenceNode.Status.Allocatable,
				Capacity:    referenceNode.Status.Capacity,
				Phase:       corev1.NodeRunning,
			},
		}
		nodes = append(nodes, node)
	}
	err := nc.CreateNodes(ctx, nodes...)
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}
	return nil
}

func extractNodePools(shoot *v1beta1.Shoot, existingNodes []corev1.Node) []api.NodePool {
	nodePools := make([]api.NodePool, len(shoot.Spec.Provider.Workers))
	for _, w := range shoot.Spec.Provider.Workers {
		workerNodes := filterExistingNodesByWorker(existingNodes, w.Name)
		np := api.NodePool{
			Name:         w.Name,
			Zones:        w.Zones,
			Max:          w.Maximum,
			Current:      int32(len(workerNodes)),
			InstanceType: w.Machine.Type,
		}
		nodePools = append(nodePools, np)
	}
	return nodePools
}

func filterExistingNodesByWorker(existingNodes []corev1.Node, workerName string) []corev1.Node {
	return lo.Filter(existingNodes, func(n corev1.Node, _ int) bool {
		poolName, ok := n.GetLabels()[poolNameLabel]
		return ok && poolName == workerName
	})
}

func getExistingNodes(ctx context.Context, gc garden.Access, shootCoord common.ShootCoordinate) ([]corev1.Node, error) {
	shootAccess, err := gc.GetShootAccess(ctx, shootCoord)
	if err != nil {
		return nil, err
	}
	return shootAccess.ListNodes(ctx)
}
