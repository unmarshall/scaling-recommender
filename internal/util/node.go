package util

import (
	"context"
	"fmt"
	"strings"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/common"

	gsc "github.com/elankath/gardener-scaling-common"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ExistingNodeInLiveClusterLabelKey = "app.kubernetes.io/existing-node"

type ReferenceNodes []corev1.Node

func (r ReferenceNodes) GetReferenceNode(instanceType string) (*corev1.Node, error) {
	filteredNodes := lo.Filter(r, func(n corev1.Node, _ int) bool {
		return GetInstanceType(n.GetLabels()) == instanceType
	})
	if len(filteredNodes) == 0 {
		return nil, fmt.Errorf("no reference node found for instance type: %s", instanceType)
	}
	return &filteredNodes[0], nil
}

func ExistingNodeInLiveCluster(node *corev1.Node) bool {
	return metav1.HasLabel(node.ObjectMeta, ExistingNodeInLiveClusterLabelKey)
}

func GetNodeNames(nodes []corev1.Node) []string {
	return lo.Map[corev1.Node, string](nodes, func(node corev1.Node, _ int) string {
		return node.Name
	})
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

func ConstructNodesFromNodeInfos(nodeInfos []api.NodeInfo) ([]*corev1.Node, error) {
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
	return nodes, nil
}

func ConstructNodeForSimRun(nodeTemplate gsc.NodeTemplate, poolName, zone string, runRef lo.Tuple2[string, string]) (*corev1.Node, error) {
	nodeNamePrefix, err := GenerateRandomString(4)
	if err != nil {
		return nil, err
	}
	nodeName := nodeNamePrefix + "-" + poolName + "-simrun-" + runRef.B
	labels := nodeTemplate.Labels
	delete(labels, "kubernetes.io/role/node")
	for key := range labels {
		if strings.HasPrefix(key, "kubernetes.io/cluster") {
			delete(labels, key)
		}
	}
	labels[common.TopologyZoneLabelKey] = zone
	labels[runRef.A] = runRef.B
	labels[common.TopologyHostLabelKey] = nodeName
	taints := []corev1.Taint{
		{Key: runRef.A, Value: runRef.B, Effect: corev1.TaintEffectNoSchedule},
	}
	taints = append(taints, nodeTemplate.Taints...)
	return doConstructNodeFromNodeTemplate(nodeTemplate, nodeName, labels, taints), nil
}

func ConstructNodeFromNodeTemplate(nodeTemplate gsc.NodeTemplate, poolName, zone string) (*corev1.Node, error) {
	nodeNamePrefix, err := GenerateRandomString(4)
	if err != nil {
		return nil, err
	}
	nodeName := nodeNamePrefix + "-" + poolName
	labels := nodeTemplate.Labels
	labels[common.TopologyZoneLabelKey] = zone
	labels[common.TopologyHostLabelKey] = nodeName
	return doConstructNodeFromNodeTemplate(nodeTemplate, nodeName, labels, nodeTemplate.Taints), nil
}

func doConstructNodeFromNodeTemplate(nodeTemplate gsc.NodeTemplate, newNodeName string, labels map[string]string, taints []corev1.Taint) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      newNodeName,
			Namespace: common.DefaultNamespace,
			Labels:    labels,
		},
		Spec: corev1.NodeSpec{
			Taints: taints,
		},
		Status: corev1.NodeStatus{
			Allocatable: nodeTemplate.Allocatable,
			Capacity:    nodeTemplate.Capacity,
			Phase:       corev1.NodeRunning,
		},
	}
}

func GetInstanceType(labels map[string]string) string {
	return labels[common.InstanceTypeLabelKey]
}
