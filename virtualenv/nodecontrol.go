package virtualenv

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/util"
)

type NodeControl interface {
	// SyncNodesFromLiveCluster syncs the nodes from the live cluster to the in-memory controlPlane. Taint if passed
	// will be set on each of the mirror nodes in the in-memory cluster.
	SyncNodesFromLiveCluster(ctx context.Context, taint *corev1.Taint, nodes ...*corev1.Node) error
	// CreateNodes creates new nodes in the in-memory controlPlane from the given node specs.
	CreateNodes(ctx context.Context, nodes ...corev1.Node) error
	// ListNodes returns the current nodes of the in-memory controlPlane.
	ListNodes(ctx context.Context, filters ...api.NodeFilter) ([]corev1.Node, error)
	// TaintNodes taints the given nodes with the given taint.
	TaintNodes(ctx context.Context, taint corev1.Taint, nodes ...*corev1.Node)
	//UnTaintNodes removes the given taint from the given nodes.
	UnTaintNodes(ctx context.Context, taintKey string, nodes ...*corev1.Node)
}

type nodeControl struct {
	client client.Client
}

func NewNodeControl(cl client.Client) NodeControl {
	return &nodeControl{
		client: cl,
	}
}

func (n nodeControl) SyncNodesFromLiveCluster(ctx context.Context, taint *corev1.Taint, nodes ...*corev1.Node) error {
	//TODO implement me
	panic("implement me")
}

func (n nodeControl) CreateNodes(ctx context.Context, nodes ...corev1.Node) error {
	//TODO implement me
	panic("implement me")
}

func (n nodeControl) ListNodes(ctx context.Context, filters ...api.NodeFilter) ([]corev1.Node, error) {
	return util.ListNodes(ctx, n.client, filters...)
}

func (n nodeControl) TaintNodes(ctx context.Context, taint corev1.Taint, nodes ...*corev1.Node) {
	//TODO implement me
	panic("implement me")
}

func (n nodeControl) UnTaintNodes(ctx context.Context, taintKey string, nodes ...*corev1.Node) {
	//TODO implement me
	panic("implement me")
}
