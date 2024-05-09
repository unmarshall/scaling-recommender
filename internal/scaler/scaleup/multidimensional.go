package scaleup

import (
	"context"
	"errors"
	"fmt"
	"github.com/samber/lo"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/rand"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/util"

	corev1 "k8s.io/api/core/v1"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

type recommender struct {
	nc       virtualenv.NodeControl
	pc       virtualenv.PodControl
	ec       virtualenv.EventControl
	pa       pricing.InstancePricingAccess
	refNodes []*corev1.Node
	state    simulationState
	logger   slog.Logger
}

type nodeScore struct {
	wasteRatio       float64
	unscheduledRatio float64
	costRatio        float64
	cumulativeScore  float64
}

type runResult struct {
	nodePoolName    string
	nodeName        string
	zone            string
	instanceType    string
	nodeScore       nodeScore
	unscheduledPods []corev1.Pod
	nodeToPods      map[string][]types.NamespacedName
	err             error
}

func errorRunResult(err error) *runResult {
	return &runResult{err: err}
}

func (r runResult) HasWinner() bool {
	return len(r.nodeToPods) > 0
}

type simulationState struct {
	originalUnscheduledPods map[string]*corev1.Pod
	existingNodes           []*corev1.Node
	unscheduledPods         []*corev1.Pod
	scheduledPods           []*corev1.Pod
	// eligibleNodePools holds the available node capacity per node pool.
	eligibleNodePools map[string]api.NodePool
}

func NewRecommender(vcp virtualenv.ControlPlane, pa pricing.InstancePricingAccess, logger slog.Logger) scaler.Recommender {
	return &recommender{
		nc:     vcp.NodeControl(),
		pc:     vcp.PodControl(),
		ec:     vcp.EventControl(),
		pa:     pa,
		logger: logger,
	}
}

func (r *recommender) Run(ctx context.Context, simReq api.SimulationRequest) scaler.Result {
	var (
		recommendations []scaler.ScaleUpRecommendation
		runNumber       int
	)
	r.initializeSimulationState(simReq)
	for {
		runNumber++
		r.logger.Info("Scale-up recommender run started...", "runNumber", runNumber)
		if len(r.state.unscheduledPods) == 0 {
			r.logger.Info("All pods are scheduled. Exiting the loop...")
			break
		}
		simRunStartTime := time.Now()
		winnerRunResult := r.runSimulation(ctx, runNumber)
		r.logger.Info("Scale-up recommender run completed", "runNumber", runNumber, "duration", time.Since(simRunStartTime).Seconds())
		if winnerRunResult.err != nil {
			r.logger.Error("runSimulation failed", "err", winnerRunResult.err)
			break
		}
		if winnerRunResult == nil {
			r.logger.Info("No winner could be identified. This will happen when no pods could be assigned. No more runs are required, exiting early", "runNumber", runNumber)
			break
		}
		recommendation := createScaleUpRecommendationFromResult(*winnerRunResult)
		if err := r.syncWinningResult(ctx, recommendation, winnerRunResult); err != nil {
			return scaler.ErrorResult(err)
		}
		r.logger.Info("For scale-up recommender", "runNumber", runNumber, "winning-score", recommendation)
		appendScaleUpRecommendation(recommendations, recommendation)
	}
	return scaler.OkScaleUpResult(recommendations)
}

func (r *recommender) initializeSimulationState(simReq api.SimulationRequest) {
	pods := util.ConstructPodsFromPodInfos(simReq.Pods, util.NilOr(simReq.PodOrder, common.SortDescending))
	nodes := util.ConstructNodesFromNodeInfos(simReq.Nodes)

	r.state.unscheduledPods, r.state.scheduledPods = util.SplitScheduledAndUnscheduledPods(pods)
	r.state.originalUnscheduledPods = lo.SliceToMap[*corev1.Pod, string, *corev1.Pod](r.state.unscheduledPods, func(pod *corev1.Pod) (string, *corev1.Pod) {
		return pod.Name, pod
	})
	r.state.eligibleNodePools = lo.SliceToMap(simReq.NodePools, func(np api.NodePool) (string, api.NodePool) {
		return np.Name, np
	})
	r.state.existingNodes = nodes
}

func (r *recommender) getReferenceNode(instanceType string) *corev1.Node {
	return lo.Filter(r.refNodes, func(n *corev1.Node, _ int) bool {
		return n.Labels[common.InstanceTypeLabelKey] == instanceType
	})[0]
}

func (r *recommender) runSimulation(ctx context.Context, runNum int) *runResult {
	var results []runResult
	resultCh := make(chan runResult, len(r.state.eligibleNodePools))
	r.triggerNodePoolSimulations(ctx, resultCh, runNum)

	// label, taint, result chan, error chan, close chan
	var errs error
	for result := range resultCh {
		if result.err != nil {
			errs = errors.Join(errs, result.err)
		} else {
			if result.HasWinner() {
				results = append(results, result)
			}
		}
	}
	if errs != nil {
		return errorRunResult(errs)
	}
	winnerRunResult := getWinningRunResult(results)
	return winnerRunResult
}

func (r *Recommender) triggerNodePoolSimulations(ctx context.Context, resultCh chan runResult, runNum int) {
	wg := &sync.WaitGroup{}
	logger := webutil.NewLogger()
	logger.Log(r.logWriter, fmt.Sprintf("Starting simulation runs for %v nodePools", maps.Keys(r.state.eligibleNodePools)))
	for _, nodePool := range r.state.eligibleNodePools {
		wg.Add(1)
		runRef := simRunRef{
			key:   "app.kubernetes.io/simulation-run",
			value: nodePool.Name + "-" + strconv.Itoa(runNum),
		}
		go r.runSimulationForNodePool(ctx, logger, wg, nodePool, resultCh, runRef)
	}
	wg.Wait()
	close(resultCh)
}

func (r *Recommender) runSimulationForNodePool(ctx context.Context, logger *webutil.Logger, wg *sync.WaitGroup, nodePool scalesim.NodePool, resultCh chan runResult, runRef simRunRef) {
	simRunStartTime := time.Now()
	defer wg.Done()
	defer func() {
		logger.Log(r.logWriter, fmt.Sprintf("Simulation run: %s for nodePool: %s completed in %f seconds", runRef.value, nodePool.Name, time.Since(simRunStartTime).Seconds()))
	}()
	defer func() {
		if err := r.cleanUpNodePoolSimRun(ctx, runRef); err != nil {
			// In the productive code, there will not be any real KAPI and ETCD. Fake API server will never return an error as everything will be in memory.
			// For now, we are only logging this error as in the POC code since the caller of recommender will re-initialize the virtual cluster.
			logger.Log(r.logWriter, "Error cleaning up simulation run: "+runRef.value+" err: "+err.Error())
		}
	}()
	var (
		node *corev1.Node
		err  error
	)
	// create a copy of all nodes and scheduled pods only
	if err = r.setupSimulationRun(ctx, runRef); err != nil {
		resultCh <- createErrorResult(err)
		return
	}
	for _, zone := range nodePool.Zones {
		if node != nil {
			if err = r.resetNodePoolSimRun(ctx, node.Name, runRef); err != nil {
				resultCh <- createErrorResult(err)
				return
			}
		}
		node, err = r.constructNodeFromExistingNodeOfInstanceType(nodePool.MachineType, nodePool.Name, zone, true, &runRef)
		if err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		if err = r.engine.VirtualClusterAccess().AddNodes(ctx, node); err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		if err = r.engine.VirtualClusterAccess().RemoveTaintFromVirtualNodes(ctx, "node.kubernetes.io/not-ready"); err != nil {
			return
		}
		deployTime := time.Now()
		unscheduledPods, err := r.createAndDeployUnscheduledPods(ctx, runRef)
		if err != nil {
			return
		}
		// in production code FAKE KAPI will not return any error. This is only for POC code where an envtest KAPI is used.
		_, _, err = simutil.WaitForAndRecordPodSchedulingEvents(ctx, r.engine.VirtualClusterAccess(), r.logWriter, deployTime, unscheduledPods, 10*time.Second)
		if err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		simRunCandidatePods, err := r.engine.VirtualClusterAccess().GetPods(ctx, "default", simutil.PodNames(unscheduledPods))
		//simRunCandidatePods, err := r.engine.VirtualClusterAccess().ListPodsMatchingLabels(ctx, runRef.asMap())
		if err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		ns := r.computeNodeScore(node, simRunCandidatePods)
		resultCh <- r.computeRunResult(nodePool.Name, nodePool.MachineType, zone, node.Name, ns, simRunCandidatePods)
	}
}

func (r *Recommender) setupSimulationRun(ctx context.Context, runRef simRunRef) error {
	// create copy of all nodes (barring existing nodes)
	clonedNodes := make([]*corev1.Node, 0, len(r.state.existingNodes))
	for _, node := range r.state.existingNodes {
		nodeCopy := node.DeepCopy()
		nodeCopy.Name = fromOriginalResourceName(nodeCopy.Name, runRef.value)
		nodeCopy.Labels[runRef.key] = runRef.value
		nodeCopy.Labels["kubernetes.io/hostname"] = nodeCopy.Name
		nodeCopy.ObjectMeta.UID = ""
		nodeCopy.ObjectMeta.ResourceVersion = ""
		nodeCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
		nodeCopy.Spec.Taints = []corev1.Taint{
			{Key: runRef.key, Value: runRef.value, Effect: corev1.TaintEffectNoSchedule},
		}
		clonedNodes = append(clonedNodes, nodeCopy)
	}
	if err := r.engine.VirtualClusterAccess().AddNodes(ctx, clonedNodes...); err != nil {
		return err
	}

	// create copy of all scheduled pods
	clonedScheduledPods := make([]corev1.Pod, 0, len(r.state.scheduledPods))
	for _, pod := range r.state.scheduledPods {
		podCopy := pod.DeepCopy()
		podCopy.Name = fromOriginalResourceName(podCopy.Name, runRef.value)
		podCopy.Labels[runRef.key] = runRef.value
		podCopy.ObjectMeta.UID = ""
		podCopy.ObjectMeta.ResourceVersion = ""
		podCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
		podCopy.Spec.Tolerations = []corev1.Toleration{
			{Key: runRef.key, Value: runRef.value, Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpEqual},
		}
		if len(podCopy.Spec.TopologySpreadConstraints) > 0 {
			updatedTSC := make([]corev1.TopologySpreadConstraint, 0, len(podCopy.Spec.TopologySpreadConstraints))
			for _, tsc := range podCopy.Spec.TopologySpreadConstraints {
				tsc.LabelSelector.MatchLabels[runRef.key] = runRef.value
				updatedTSC = append(updatedTSC, tsc)
			}
			podCopy.Spec.TopologySpreadConstraints = updatedTSC
		}
		podCopy.Spec.NodeName = fromOriginalResourceName(podCopy.Spec.NodeName, runRef.value)
		clonedScheduledPods = append(clonedScheduledPods, *podCopy)
	}
	if err := r.engine.VirtualClusterAccess().AddPods(ctx, clonedScheduledPods...); err != nil {
		return err
	}
	return nil
}

func (r *Recommender) resetNodePoolSimRun(ctx context.Context, nodeName string, runRef simRunRef) error {
	// remove the node with the nodeName
	if err := r.engine.VirtualClusterAccess().DeleteNode(ctx, nodeName); err != nil {
		return err
	}
	// remove the pods with nodeName
	pods, err := r.engine.VirtualClusterAccess().ListPodsMatchingLabels(ctx, runRef.asMap())
	if err != nil {
		return err
	}
	return r.engine.VirtualClusterAccess().DeletePods(ctx, pods...)
}

func getWinningRunResult(results []runResult) *runResult {
	if len(results) == 0 {
		return nil
	}
	var winner runResult
	minScore := math.MaxFloat64
	var winningRunResults []runResult
	for _, v := range results {
		if v.nodeScore.cumulativeScore < minScore {
			winner = v
			minScore = v.nodeScore.cumulativeScore
		}
	}
	for _, v := range results {
		if v.nodeScore.cumulativeScore == minScore {
			winningRunResults = append(winningRunResults, v)
		}
	}
	rand.Seed(uint64(time.Now().UnixNano()))
	winningIndex := rand.Intn(len(winningRunResults))
	winner = winningRunResults[winningIndex]
	return &winner
}

func createScaleUpRecommendationFromResult(result runResult) scaler.ScaleUpRecommendation {
	return scaler.ScaleUpRecommendation{
		Zone:         result.zone,
		NodePoolName: result.nodePoolName,
		IncrementBy:  int32(1),
		InstanceType: result.instanceType,
	}
}

func appendScaleUpRecommendation(recommendations []scaler.ScaleUpRecommendation, recommendation scaler.ScaleUpRecommendation) {
	var found bool
	for i, r := range recommendations {
		if r.NodePoolName == recommendation.NodePoolName {
			r.IncrementBy += recommendation.IncrementBy
			found = true
			recommendations[i] = r
		}
	}
	if !found {
		recommendations = append(recommendations, recommendation)
	}
}

func (r *recommender) syncWinningResult(ctx context.Context, recommendation interface{}, result interface{}) error {

}
