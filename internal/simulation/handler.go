package simulation

import (
	"context"
	"errors"
	"fmt"
	gsc "github.com/elankath/gardener-scaling-common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"log/slog"
	"net/http"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/simulation/web"
	"unmarshall/scaling-recommender/internal/util"
)

type Handler struct {
	engine Engine
}

func NewSimulationHandler(engine Engine) *Handler {
	return &Handler{
		engine: engine,
	}
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	defer func() {
		err := r.Body.Close()
		if err != nil {
			slog.Info("error closing request body", "error", err)
		}
	}()

	// first clean up the virtual cluster
	if err := h.engine.VirtualControlPlane().FactoryReset(r.Context()); err != nil {
		web.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	clusterSnapshot, err := web.ParseClusterSnapshot(r.Body)
	if err != nil {
		web.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	simRequest, err := h.createSimulationRequest(r.Context(), clusterSnapshot)
	if err != nil {
		web.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	baseLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger := baseLogger.With("id", simRequest.ID)
	logger.Info("received simulation request", "request", simRequest.ID)

	recommender := h.engine.RecommenderFactory().GetRecommender(scaler.DefaultScaleUpAlgo)
	startTime := time.Now()
	result := recommender.Run(r.Context(), h.engine.GetScorer(), simRequest)
	if result.IsError() {
		slog.Error("Error in running simulation", result.Err.Error())
		web.ErrorResponse(w, http.StatusInternalServerError, result.Err.Error())
		return
	}
	if err = h.applyRecommendation(r.Context(), result.Ok.Recommendation.ScaleUp, simRequest.NodeTemplates); err != nil {
		slog.Error("Failed in applying recommendation")
		web.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	runTime := time.Since(startTime)
	response := api.RecommendationResponse{
		Recommendation:  result.Ok.Recommendation,
		UnscheduledPods: result.Ok.UnscheduledPods,
		RunTime:         fmt.Sprintf("%d millis", runTime.Milliseconds()),
	}
	if err = web.WriteJSON(w, http.StatusOK, response); err != nil {
		web.ErrorResponse(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) applyRecommendation(ctx context.Context, recommendations []api.ScaleUpRecommendation, nodeTemplates map[string]gsc.NodeTemplate) error {
	targetClient := h.engine.TargetClient()
	var nodesToCreate []*corev1.Node
	for _, r := range recommendations {
		slog.Info("Applying recommendation", "nodePool", r.NodePoolName, "zone", r.Zone, "instanceType", r.InstanceType, "incrementBy", r.IncrementBy)
		for i := int32(0); i < r.IncrementBy; i++ {
			//nodeTemplate := nodeTemplates[r.InstanceType]
			nodeTemplate := util.FindNodeTemplate(nodeTemplates, r.NodePoolName, r.Zone)
			if nodeTemplate == nil {
				return fmt.Errorf("node template not found for instance type %s", r.InstanceType)
			}
			node, err := util.ConstructNodeFromNodeTemplate(*nodeTemplate, r.NodePoolName, r.Zone)
			if err != nil {
				return err
			}
			nodesToCreate = append(nodesToCreate, node)
		}
	}
	return createAndUntaintNodes(ctx, targetClient, nodesToCreate)
}

func createAndUntaintNodes(ctx context.Context, cl client.Client, nodes []*corev1.Node) error {
	if err := createNodes(ctx, cl, nodes); err != nil {
		return err
	}
	return untaintNodes(ctx, cl, common.NotReadyTaintKey, nodes)
}

func createNodes(ctx context.Context, cl client.Client, nodes []*corev1.Node) error {
	var errs error
	for _, node := range nodes {
		node.ObjectMeta.ResourceVersion = ""
		node.ObjectMeta.UID = ""
		errs = errors.Join(errs, cl.Create(ctx, node))
	}
	return errs
}

func untaintNodes(ctx context.Context, cl client.Client, taintKey string, nodes []*corev1.Node) error {
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

func (h *Handler) createSimulationRequest(ctx context.Context, cs *gsc.ClusterSnapshot) (simRequest api.SimulationRequest, err error) {
	simRequest.ID = cs.ID
	for _, pc := range cs.PriorityClasses {
		simRequest.PriorityClasses = append(simRequest.PriorityClasses, pc.PriorityClass)
	}
	var podList corev1.PodList
	targetClient := h.engine.TargetClient()
	if err = targetClient.List(ctx, &podList); err != nil {
		err = fmt.Errorf("[createSimulationRequest] failed to list pods in target cluster: %w", err)
		return
	}
	for _, p := range podList.Items {
		if p.Namespace != "kube-system" {
			pod := api.PodInfo{
				Name:              p.Name,
				Labels:            p.Labels,
				Spec:              p.Spec,
				NominatedNodeName: p.Status.NominatedNodeName,
				Count:             1,
			}
			simRequest.Pods = append(simRequest.Pods, pod)
		}
	}
	nodeCountPerPool := deriveNodeCountPerWorkerPool(cs.Nodes)
	nodePools := make([]api.NodePool, 0, len(cs.WorkerPools))
	nodeTemplates := cs.AutoscalerConfig.NodeTemplates
	//nodeTemplates := make(map[string]gsc.NodeTemplate, len(cs.WorkerPools))
	for _, wp := range cs.WorkerPools {
		count := nodeCountPerPool[wp.Name]
		nodePool := api.NodePool{
			Name:         wp.Name,
			Zones:        sets.New(wp.Zones...),
			Max:          int32(wp.Maximum),
			Current:      int32(count),
			InstanceType: wp.MachineType,
		}
		nodePools = append(nodePools, nodePool)
		simRequest.NodePools = nodePools

		//nodeTemplate := FindNodeTemplateForInstanceType(wp.MachineType, cs.AutoscalerConfig.NodeTemplates)
		//if nodeTemplate == nil {
		//	err = fmt.Errorf("createSimulationRequest cannot find node template for workerpool %q", wp.Name)
		//	return
		//}
		////computeRevisedResourcesForNodeTemplate(nodeTemplate, maxResourceList)
		//nodeTemplates[wp.MachineType] = *nodeTemplate
	}
	simRequest.NodeTemplates = nodeTemplates

	for _, n := range cs.Nodes {
		//nodeTemplate, ok := nodeTemplates[n.Labels[common.InstanceTypeLabelKey]]
		nodeTemplate := util.FindNodeTemplateForInstanceType(n.Labels[common.InstanceTypeLabelKey], simRequest.NodeTemplates)
		if nodeTemplate == nil {
			err = fmt.Errorf("createSimulationRequest cannot find node template for node %q", n.Name)
			return
		}
		//if !ok {
		//	err = fmt.Errorf("createSimulationRequest cannot find node template for node %q", n.Name)
		//	return
		//}
		node := api.NodeInfo{
			Name:        n.Name,
			Labels:      n.Labels,
			Taints:      n.Taints,
			Allocatable: nodeTemplate.Allocatable,
			Capacity:    n.Capacity,
		}
		simRequest.Nodes = append(simRequest.Nodes, node)
	}
	return
}

func deriveNodeCountPerWorkerPool(nodes []gsc.NodeInfo) map[string]int {
	nodeCountPerPool := make(map[string]int)
	for _, n := range nodes {
		nodeCountPerPool[n.Labels[common.WorkerPoolLabelKey]]++

	}
	return nodeCountPerPool
}
