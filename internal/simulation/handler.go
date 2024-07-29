package simulation

import (
	"fmt"
	"github.com/elankath/gardener-scaling-common"
	"k8s.io/apimachinery/pkg/util/sets"
	"log/slog"
	"net/http"
	"os"
	"time"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/scaler/scorer"
	"unmarshall/scaling-recommender/internal/simulation/web"
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

	simRequest, err := createSimulationRequest(clusterSnapshot)
	if err != nil {
		web.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	baseLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger := baseLogger.With("id", simRequest.ID)
	logger.Info("received simulation request", "request", simRequest.ID)

	recommender := h.engine.RecommenderFactory().GetRecommender(scaler.MultiDimensionScoringScaleUpAlgo)
	startTime := time.Now()
	nodeScorer, err := scorer.NewScorer(h.engine.ScoringStrategy(), h.engine.PricingAccess(), simRequest.NodePools)
	if err != nil {
		web.ErrorResponse(w, http.StatusInternalServerError, err.Error())
	}
	result := recommender.Run(r.Context(), nodeScorer, simRequest)
	if result.IsError() {
		web.ErrorResponse(w, http.StatusInternalServerError, result.Err.Error())
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

func createSimulationRequest(cs *gsc.ClusterSnapshot) (simRequest api.SimulationRequest, err error) {
	simRequest.ID = cs.ID
	for _, pc := range cs.PriorityClasses {
		simRequest.PriorityClasses = append(simRequest.PriorityClasses, pc.PriorityClass)
	}
	for _, pi := range cs.Pods {
		pod := api.PodInfo{
			Name:              pi.Name,
			Labels:            pi.Labels,
			Spec:              pi.Spec,
			NominatedNodeName: pi.NominatedNodeName,
			Count:             1,
		}
		simRequest.Pods = append(simRequest.Pods, pod)
	}
	for _, n := range cs.Nodes {
		node := api.NodeInfo{
			Name:        n.Name,
			Labels:      n.Labels,
			Taints:      n.Taints,
			Allocatable: n.Allocatable,
			Capacity:    n.Capacity,
		}
		simRequest.Nodes = append(simRequest.Nodes, node)
	}
	nodeCountPerPool := deriveNodeCountPerWorkerPool(cs.Nodes)
	nodePools := make([]api.NodePool, 0, len(cs.WorkerPools))

	for _, wp := range cs.WorkerPools {
		count, ok := nodeCountPerPool[wp.Name]
		if !ok {
			err = fmt.Errorf("createSimulationRequest cannot find workerpool with name %q", wp.Name)
			return
		}
		nodePool := api.NodePool{
			Name:         wp.Name,
			Zones:        sets.New(wp.Zones...),
			Max:          int32(wp.Maximum),
			Current:      int32(count),
			InstanceType: wp.MachineType,
		}
		nodePools = append(nodePools, nodePool)
		simRequest.NodePools = nodePools

		nodeTemplate := findNodeTemplate(wp.MachineType, cs.AutoscalerConfig.NodeTemplates)
		if nodeTemplate == nil {
			err = fmt.Errorf("createSimulationRequest cannot find node template for workerpool %q", wp.Name)
			return
		}
		simRequest.NodeTemplates[wp.MachineType] = *nodeTemplate
	}
	return
}

/*
	systemComponentResources = get system pods resourceList from existing pods
	Allocatable = Capacity - (kube-reserved + system-reserved + systemComponentResources)
*/

func findNodeTemplate(instanceType string, csNodeTemplates map[string]gsc.NodeTemplate) *api.NodeTemplate {
	for _, nt := range csNodeTemplates {
		if nt.InstanceType == instanceType {
			return &api.NodeTemplate{
				InstanceType: instanceType,
				Labels:       nt.Labels,
				Capacity:     nt.Capacity,
			}
		}
	}
	return nil
}

func deriveNodeCountPerWorkerPool(nodes []gsc.NodeInfo) map[string]int {
	nodeCountPerPool := make(map[string]int)
	for _, n := range nodes {
		nodeCountPerPool[n.Labels["worker.gardener.cloud/pool"]]++
	}
	return nodeCountPerPool
}
