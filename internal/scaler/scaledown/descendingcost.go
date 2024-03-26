package scaledown

import (
	"cmp"
	"context"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/simulation/web"
	"unmarshall/scaling-recommender/internal/util"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

const (
	podScheduledEventsTimeout = 10 * time.Second
)

type recommender struct {
	nc virtualenv.NodeControl
	pc virtualenv.PodControl
	ec virtualenv.EventControl
	pa pricing.InstancePricingAccess
}

func NewDescendingCostRecommender(nc virtualenv.NodeControl, pc virtualenv.PodControl, ec virtualenv.EventControl, pricingAccess pricing.InstancePricingAccess) scaler.Recommender {
	return &recommender{
		nc: nc,
		pc: pc,
		ec: ec,
		pa: pricingAccess,
	}
}

func (r *recommender) Run(ctx context.Context, w scaler.LogWriterFlusher) scaler.Result {
	startTime := time.Now()
	defer func() {
		runDuration := time.Since(startTime)
		web.Logf(w, "Descending cost scale down recommender took %f seconds", runDuration.Seconds())
	}()
	nodes, err := r.getAndSortNodesByDescendingCost()
	if err != nil {
		web.InternalError(w, err)
	}
	deletableNodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		web.Logf(w, "Considering candidate node %s...", node.Name)
		assignedPods, err := r.getPodsAssignedToNode(ctx, node.Name)
		if err != nil {
			web.InternalError(w, err)
			return scaler.ErrorResult(err)
		}
		if err = r.deleteNodeAndAssignedPods(ctx, w, &node, assignedPods); err != nil {
			web.InternalError(w, err)
			return scaler.ErrorResult(err)
		}
		if len(assignedPods) == 0 {
			deletableNodeNames = append(deletableNodeNames, node.Name)
			web.Logf(w, "Node %s has no assigned pods and can be deleted", node.Name)
		}
		deployStartTime := time.Now()
		adjustedPods, err := r.createAndDeployNewUnassignedPods(ctx, w, assignedPods)
		if err != nil {
			web.InternalError(w, err)
			return scaler.ErrorResult(err)
		}
		scheduledPodNames, unscheduledPodNames, err := util.WaitForAndRecordPodSchedulingEvents(ctx, r.ec, deployStartTime, adjustedPods, podScheduledEventsTimeout)
		if err != nil {
			return scaler.ErrorResult(err)
		}
		web.Logf(w, "Scheduled pods: %v, unscheduled pods: %v", scheduledPodNames, unscheduledPodNames)
		if len(unscheduledPodNames) != 0 {
			web.Logf(w, "Node %s CANNOT be removed since it will result in %d unscheduled pods", node.Name, len(unscheduledPodNames))
			if err = r.pc.DeletePods(ctx, adjustedPods...); err != nil {
				web.InternalError(w, err)
				return scaler.ErrorResult(err)
			}
			web.Logf(w, "Recreating node %s and corresponding pods %s", node.Name, util.GetPodNames(assignedPods))
			if err = r.recreateNodeWithPods(ctx, &node, assignedPods); err != nil {
				web.InternalError(w, err)
				return scaler.ErrorResult(err)
			}
		} else {
			web.Logf(w, "Node %s can be removed, adding it to deletion candidates", node.Name)
			deletableNodeNames = append(deletableNodeNames, node.Name)
		}
	}
	return scaler.OkResult(scaler.NewScaleDownRecommendation(deletableNodeNames))
}

func (r *recommender) recreateNodeWithPods(ctx context.Context, node *corev1.Node, pods []corev1.Pod) error {
	if err := r.nc.CreateNodes(ctx, *node); err != nil {
		return err
	}
	newPods, err := util.ConstructNewPods(pods, node.Name, common.BinPackingSchedulerName)
	if err != nil {
		return err
	}
	return r.pc.CreatePods(ctx, newPods...)
}

func (r *recommender) createAndDeployNewUnassignedPods(ctx context.Context, w scaler.LogWriterFlusher, assignedPods []corev1.Pod) ([]corev1.Pod, error) {
	clonedUnassignedPods, err := util.ConstructNewPods(assignedPods, "", common.BinPackingSchedulerName)
	if err != nil {
		web.InternalError(w, err)
		return clonedUnassignedPods, err
	}
	web.Logf(w, "Deploying new unassigned pods %v", util.GetPodNames(clonedUnassignedPods))
	return clonedUnassignedPods, r.pc.CreatePods(ctx, clonedUnassignedPods...)
}

func (r *recommender) deleteNodeAndAssignedPods(ctx context.Context, w scaler.LogWriterFlusher, node *corev1.Node, pods []corev1.Pod) error {
	web.Logf(w, "Deleting candidate node %s", node.Name)
	if err := r.nc.DeleteNodes(ctx, node.Name); err != nil {
		return err
	}
	web.Logf(w, "Deleting assigned pods %v", util.GetPodNames(pods))
	return r.pc.DeletePods(ctx, pods...)
}

// TODO one should filter the system pods either via explicitly captured names or via a set of reserved namespaces.
func (r *recommender) getPodsAssignedToNode(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
	return r.pc.ListPods(ctx, func(pod *corev1.Pod) bool {
		return pod.Spec.NodeName == nodeName
	})
}

func (r *recommender) getAndSortNodesByDescendingCost() ([]corev1.Node, error) {
	nodes, err := r.nc.ListNodes(context.Background())
	if err != nil {
		return nil, err
	}
	slices.SortFunc(nodes, r.comparePriceDescending)
	return nodes, nil
}

func (r *recommender) comparePriceDescending(n1, n2 corev1.Node) int {
	n1Price := r.pa.Get3YearReservedPricing(util.GetNodeInstanceType(n1))
	n2Price := r.pa.Get3YearReservedPricing(util.GetNodeInstanceType(n2))
	return -cmp.Compare(n1Price, n2Price)
}
