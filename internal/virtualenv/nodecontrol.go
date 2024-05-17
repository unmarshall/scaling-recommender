package virtualenv

import (
	"context"
	"errors"
	"log/slog"

	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/util"
)

type NodeControl interface {
	// CreateNodes creates new nodes in the in-memory controlPlane from the given node specs.
	CreateNodes(ctx context.Context, nodes ...*corev1.Node) error
	// GetNode return the node matching object key
	GetNode(ctx context.Context, objectKey types.NamespacedName) (*corev1.Node, error)
	// ListNodes returns the current nodes of the in-memory controlPlane.
	ListNodes(ctx context.Context, filters ...common.NodeFilter) ([]corev1.Node, error)
	// TaintNodes taints the given nodes with the given taint.
	TaintNodes(ctx context.Context, taint corev1.Taint, nodes ...*corev1.Node) error
	//UnTaintNodes removes the given taint from the given nodes.
	UnTaintNodes(ctx context.Context, taintKey string, nodes ...*corev1.Node) error
	// DeleteNodes deletes the nodes identified by the given names from the in-memory controlPlane.
	DeleteNodes(ctx context.Context, nodeNames ...string) error
	// DeleteAllNodes deletes all nodes from the in-memory controlPlane.
	DeleteAllNodes(ctx context.Context) error
	// DeleteNodesMatchingLabels deletes all nodes matching labels
	DeleteNodesMatchingLabels(ctx context.Context, labels map[string]string) error
}

type nodeControl struct {
	client client.Client
}

func NewNodeControl(cl client.Client) NodeControl {
	return &nodeControl{
		client: cl,
	}
}

func (n nodeControl) CreateNodes(ctx context.Context, nodes ...*corev1.Node) error {
	var errs error
	for _, node := range nodes {
		node.ObjectMeta.ResourceVersion = ""
		node.ObjectMeta.UID = ""
		errs = errors.Join(errs, n.client.Create(ctx, node))
	}
	return errs
}

func (n nodeControl) GetNode(ctx context.Context, objectKey types.NamespacedName) (*corev1.Node, error) {
	node := corev1.Node{}
	err := n.client.Get(ctx, objectKey, &node)
	return &node, err
}

func (n nodeControl) ListNodes(ctx context.Context, filters ...common.NodeFilter) ([]corev1.Node, error) {
	return util.ListNodes(ctx, n.client, filters...)
}

func (n nodeControl) TaintNodes(ctx context.Context, taint corev1.Taint, nodes ...*corev1.Node) error {
	var errs error
	failedToPatchNodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		patch := client.MergeFromWithOptions(node.DeepCopy(), client.MergeFromWithOptimisticLock{})
		node.Spec.Taints = append(node.Spec.Taints, taint)
		if err := n.client.Patch(ctx, node, patch); err != nil {
			failedToPatchNodeNames = append(failedToPatchNodeNames, node.Name)
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		slog.Error("failed to patch one or more nodes with taint", "taint", taint, "nodes", failedToPatchNodeNames, "error", errs)
	}
	return errs
}

func (n nodeControl) UnTaintNodes(ctx context.Context, taintKey string, nodes ...*corev1.Node) error {
	var errs error
	failedToPatchNodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		patch := client.MergeFromWithOptions(node.DeepCopy(), client.MergeFromWithOptimisticLock{})
		var newTaints []corev1.Taint
		for _, taint := range node.Spec.Taints {
			if taint.Key != taintKey {
				newTaints = append(newTaints, taint)
			}
		}
		node.Spec.Taints = newTaints
		if err := n.client.Patch(ctx, node, patch); err != nil {
			failedToPatchNodeNames = append(failedToPatchNodeNames, node.Name)
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		slog.Error("failed to remove taint from nodes", "taint", taintKey, "nodes", failedToPatchNodeNames, "error", errs)
	}
	return errs
}

func (n nodeControl) DeleteNodes(ctx context.Context, nodeNames ...string) error {
	var errs error
	targetNodes, err := n.ListNodes(ctx, func(node *corev1.Node) bool {
		return slices.Contains(nodeNames, node.Name)
	})
	if err != nil {
		return err
	}
	for _, node := range targetNodes {
		errs = errors.Join(errs, n.client.Delete(ctx, &node))
	}
	return errs
}

func (n nodeControl) DeleteAllNodes(ctx context.Context) error {
	return n.client.DeleteAllOf(ctx, &corev1.Node{})
}

func (n nodeControl) DeleteNodesMatchingLabels(ctx context.Context, labels map[string]string) error {
	return n.client.DeleteAllOf(ctx, &corev1.Node{}, client.MatchingLabels(labels))
}
