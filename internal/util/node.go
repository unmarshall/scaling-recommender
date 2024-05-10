package util

import (
	"context"

	"unmarshall/scaling-recommender/api"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/common"
)

const ExistingNodeInLiveClusterLabelKey = "app.kubernetes.io/existing-node"

func ExistingNodeInLiveCluster(node *corev1.Node) bool {
	return metav1.HasLabel(node.ObjectMeta, ExistingNodeInLiveClusterLabelKey)
}

func GetNodeNames(nodes []corev1.Node) []string {
	return lo.Map[corev1.Node, string](nodes, func(node corev1.Node, _ int) string {
		return node.Name
	})
}

func GetNodeInstanceType(node corev1.Node) string {
	return node.Labels["node.kubernetes.io/instance-type"]
}

func ListNodes(ctx context.Context, cl client.Client, filters ...common.NodeFilter) ([]corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := cl.List(ctx, nodes)
	if err != nil {
		return nil, err
	}
	if filters == nil {
		return nodes.Items, nil
	}
	filteredNodes := make([]corev1.Node, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		if ok := evaluateNodeFilters(&n, filters); ok {
			filteredNodes = append(filteredNodes, n)
		}
	}
	return filteredNodes, nil
}

func evaluateNodeFilters(node *corev1.Node, filters []common.NodeFilter) bool {
	for _, f := range filters {
		if ok := f(node); !ok {
			return false
		}
	}
	return true
}

func ConstructNodesFromNodeInfos(nodeInfos []api.NodeInfo) []*corev1.Node {
	nodes := make([]*corev1.Node, 0, len(nodeInfos))
	for _, np := range nodeInfos {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      np.Name,
				Namespace: "default",
				Labels:    np.Labels,
			},
			Spec: corev1.NodeSpec{
				Taints: np.Taints,
			},
			Status: corev1.NodeStatus{
				Allocatable: np.Allocatable,
				Capacity:    np.Capacity,
				Phase:       corev1.NodeRunning,
			},
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func ConstructNodeForSimRun(refNode *corev1.Node, poolName, zone string, runRef lo.Tuple2[string, string]) (*corev1.Node, error) {
	nodeNamePrefix, err := GenerateRandomString(4)
	if err != nil {
		return nil, err
	}
	nodeName := nodeNamePrefix + "-" + poolName + "-simrun-" + runRef.B
	nodeLabels := refNode.Labels
	nodeLabels[common.TopologyZoneLabelKey] = zone
	nodeLabels[runRef.A] = runRef.B
	nodeLabels["kubernetes.io/hostname"] = nodeName

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: "default",
			Labels:    nodeLabels,
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: runRef.A, Value: runRef.B, Effect: corev1.TaintEffectNoSchedule},
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: refNode.Status.Allocatable,
			Capacity:    refNode.Status.Capacity,
			Phase:       corev1.NodeRunning,
		},
	}
	return node, nil
}

func GetInstanceType(node *corev1.Node) string {
	return node.Labels[common.InstanceTypeLabelKey]
}
