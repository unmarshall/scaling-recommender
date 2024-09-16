package util

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/exp/maps"
	"log/slog"
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

func ConstructNodesFromNodeInfos(nodeInfos []api.NodeInfo, nodeTemplates map[string]gsc.NodeTemplate) ([]*corev1.Node, error) {
	nodes := make([]*corev1.Node, 0, len(nodeInfos))
	for _, np := range nodeInfos {
		//nodeTemplate, ok := nodeTemplates[np.Labels[common.InstanceTypeLabelKey]]
		//if !ok {
		//	return nil, fmt.Errorf("no template found for instance type %s", np.Labels[common.InstanceTypeLabelKey])
		//}
		nodeTemplate := FindNodeTemplateForInstanceType(np.Labels[common.InstanceTypeLabelKey], nodeTemplates)
		if nodeTemplate == nil {
			return nil, fmt.Errorf("no template found for instance type %s", np.Labels[common.InstanceTypeLabelKey])
		}
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
				Allocatable: nodeTemplate.Allocatable,
				Capacity:    np.Capacity,
				Phase:       corev1.NodeRunning,
			},
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}
func CreateAndUntaintNodes(ctx context.Context, cl client.Client, nodes []*corev1.Node) error {
	if err := CreateNodes(ctx, cl, nodes); err != nil {
		return err
	}
	return UntaintNodes(ctx, cl, common.NotReadyTaintKey, nodes)
}

func CreateNodes(ctx context.Context, cl client.Client, nodes []*corev1.Node) error {
	var errs error
	for _, node := range nodes {
		node.ObjectMeta.ResourceVersion = ""
		node.ObjectMeta.UID = ""
		errs = errors.Join(errs, cl.Create(ctx, node))

	}
	return errs
}

func UntaintNodes(ctx context.Context, cl client.Client, taintKey string, nodes []*corev1.Node) error {
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
		if err := cl.Patch(ctx, node, patch); err != nil {
			failedToPatchNodeNames = append(failedToPatchNodeNames, node.Name)
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		slog.Error("failed to remove taint from nodes", "taint", taintKey, "nodes", failedToPatchNodeNames, "error", errs)
	}
	return errs
}

func FindNodeTemplate(nodeTemplates map[string]gsc.NodeTemplate, poolName, zone string) *gsc.NodeTemplate {
	for _, nt := range nodeTemplates {
		if nt.Zone == zone && nt.Labels[common.WorkerPoolLabelKey] == poolName {
			return &nt
		}
	}
	return nil
}

func FindNodeTemplateForInstanceType(instanceType string, csNodeTemplates map[string]gsc.NodeTemplate) *gsc.NodeTemplate {
	for _, nt := range csNodeTemplates {
		if nt.InstanceType == instanceType {
			return &nt
		}
	}
	return nil
}

func ConstructNodeForSimRun(nodeTemplate gsc.NodeTemplate, poolName, zone string, runRef lo.Tuple2[string, string]) (*corev1.Node, error) {
	nodeNamePrefix, err := GenerateRandomString(4)
	if err != nil {
		return nil, err
	}
	nodeName := nodeNamePrefix + "-" + poolName + "-sr-" + runRef.B
	labels := maps.Clone(nodeTemplate.Labels)
	delete(labels, "kubernetes.io/role/node")
	for key := range labels {
		if strings.HasPrefix(key, "kubernetes.io/cluster") {
			delete(labels, key)
		}
	}
	labels[common.TopologyZoneLabelKey] = zone
	labels[runRef.A] = runRef.B
	labels[common.TopologyHostLabelKey] = nodeName
	//labels[common.WorkerPoolLabelKey] = poolName
	//labels[common.GKETopologyLabelKey] = zone
	//labels[common.AWSTopologyLabelKey] = zone
	//labels[common.FailureDomainLabelKey] = zone
	//labels[common.WorkerGroupLabelKey] = poolName
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
	labels := maps.Clone(nodeTemplate.Labels)
	labels[common.TopologyZoneLabelKey] = zone
	labels[common.TopologyHostLabelKey] = nodeName
	//labels[common.WorkerPoolLabelKey] = poolName
	//labels[common.GKETopologyLabelKey] = zone
	//labels[common.AWSTopologyLabelKey] = zone
	//labels[common.FailureDomainLabelKey] = zone
	//labels[common.WorkerGroupLabelKey] = poolName

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
