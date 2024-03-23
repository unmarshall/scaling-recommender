package util

import (
	"context"

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
