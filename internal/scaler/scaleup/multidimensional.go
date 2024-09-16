package scaleup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	gsc "github.com/elankath/gardener-scaling-common"
	v1 "k8s.io/api/scheduling/v1"

	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/util"

	"github.com/samber/lo"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/scaler"

	corev1 "k8s.io/api/core/v1"

	kvclapi "github.com/unmarshall/kvcl/api"
	kvcl "github.com/unmarshall/kvcl/pkg/control"
)

const (
	simRunKey          = "app.kubernetes.io/simulation-run"
	resourceNameFormat = "%s-sr-%s"
)

type recommender struct {
	nc             kvclapi.NodeControl
	pc             kvclapi.PodControl
	ec             kvclapi.EventControl
	pa             pricing.InstancePricingAccess
	client         client.Client
	scorer         scaler.Scorer
	state          simulationState
	nodeTemplates  map[string]gsc.NodeTemplate
	logger         *slog.Logger
	resultLogsPath string
}

type nodeUtilisationInfo struct {
	Zone              string              `json:"zone,omitempty"`
	NodePoolName      string              `json:"node_pool_name,omitempty"`
	Pods              []string            `json:"pods"`
	ResourcesConsumed corev1.ResourceList `json:"resources_consumed"`
	Capacity          corev1.ResourceList `json:"capacity,omitempty"`
}

type recommenderRunResult struct {
	NodeUtilInfos   map[string]nodeUtilisationInfo `json:"node_util_infos,omitempty"`
	UnscheduledPods []string                       `json:"unscheduled_pods,omitempty"`
}

type podResourceInfo struct {
	name    string
	request corev1.ResourceList
}

func (pri podResourceInfo) String() string {
	return fmt.Sprintf("{name: %s, cpu: %s memory: %s}", pri.name, pri.request.Cpu().String(), pri.request.Memory().String())
}

type runResult struct {
	nodePoolName    string
	nodeName        string
	zone            string
	instanceType    string
	nodeScore       float64
	unscheduledPods []*corev1.Pod
	nodeToPods      map[string][]podResourceInfo
	nodeCapacity    corev1.ResourceList
	err             error
	logs            []string
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
	priorityClasses   []v1.PriorityClass
}

func (s *simulationState) updateEligibleNodePools(recommendation *api.ScaleUpRecommendation) {
	np, ok := s.eligibleNodePools[recommendation.NodePoolName]
	if !ok {
		return
	}
	np.Current += recommendation.IncrementBy
	if np.Current == np.Max {
		delete(s.eligibleNodePools, recommendation.NodePoolName)
	} else {
		s.eligibleNodePools[recommendation.NodePoolName] = np
	}
}

func (s *simulationState) getUnscheduledPodObjectKeys() []client.ObjectKey {
	objKeys := make([]client.ObjectKey, 0, len(s.unscheduledPods))
	for _, pod := range s.unscheduledPods {
		objKeys = append(objKeys, client.ObjectKeyFromObject(pod))
	}
	return objKeys
}

func NewRecommender(vcp kvclapi.ControlPlane, baseLogger *slog.Logger) scaler.Recommender {
	return &recommender{
		nc:     vcp.NodeControl(),
		pc:     vcp.PodControl(),
		ec:     vcp.EventControl(),
		client: vcp.Client(),
		logger: baseLogger,
	}
}

func (r *recommender) Run(ctx context.Context, scorer scaler.Scorer, simReq api.SimulationRequest) scaler.Result {
	var (
		recommendations   []api.ScaleUpRecommendation
		runNumber         int
		recommenderResult recommenderRunResult
	)
	nodeUtilisationInfos := make(map[string]nodeUtilisationInfo)
	resultLogsDir, err := makeResultsLogDir()
	if err != nil {
		return scaler.Result{Err: err}
	}
	resultsLogPath := filepath.Join(resultLogsDir, fmt.Sprintf("%s-results.log", simReq.ID))
	resultsLogFile, err := os.OpenFile(resultsLogPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	defer func() {
		if err = resultsLogFile.Close(); err != nil {
			r.logger.Error("Failed to close results log file", "error", err)
		}
	}()
	r.resultLogsPath = resultsLogPath
	r.scorer = scorer
	r.nodeTemplates = simReq.NodeTemplates
	if err := r.initializeSimulationState(simReq); err != nil {
		return scaler.ErrorResult(err)
	}
	if err := r.initializeVirtualCluster(ctx); err != nil {
		return scaler.ErrorResult(err)
	}
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
		if winnerRunResult == nil {
			r.logger.Info("No winner could be identified. This will happen when no pods could be assigned. No more runs are required, exiting early", "runNumber", runNumber)
			break
		}
		if winnerRunResult.err != nil {
			r.logger.Error("runSimulation failed", "err", winnerRunResult.err)
			break
		}
		recommendation := createScaleUpRecommendationFromResult(*winnerRunResult)
		if err := r.syncWinningResult(ctx, &recommendation, winnerRunResult); err != nil {
			return scaler.ErrorResult(err)
		}
		nodeUtilisationInfos = appendNodeUtilisationInfo(*winnerRunResult, nodeUtilisationInfos)
		r.writeWinningResult(winnerRunResult, resultsLogFile)
		r.logger.Info("For scale-up recommender", "runNumber", runNumber, "winning-score", recommendation)
		recommendations = appendScaleUpRecommendation(recommendations, recommendation)
	}
	recommenderRunResultLogPath := filepath.Join(resultLogsDir, fmt.Sprintf("%s-util-info.json", simReq.ID))
	recommenderResult = recommenderRunResult{
		NodeUtilInfos:   nodeUtilisationInfos,
		UnscheduledPods: util.GetPodNames(r.state.unscheduledPods),
	}
	r.writeRecommenderRunResults(recommenderResult, recommenderRunResultLogPath)
	return scaler.OkScaleUpResult(recommendations, r.state.getUnscheduledPodObjectKeys())
}

func (r *recommender) writeRecommenderRunResults(recommenderResult recommenderRunResult, resultsLogPath string) {
	bytes, err := json.Marshal(recommenderResult)
	if err != nil {
		r.logger.Error("Failed to marshal recommender run result", "error", err)
	}
	if err = os.WriteFile(resultsLogPath, bytes, 0644); err != nil {
		r.logger.Error("Failed to write recommender run result to file", "error", err)
	}
}

func (r *recommender) writeWinningResult(result *runResult, file *os.File) {
	if _, err := file.WriteString(fmt.Sprintf("NodePool: %s, Node: %s, Zone: %s, InstanceType: %s, ScheduledPods: %v\n", result.nodePoolName, result.nodeName, result.zone, result.instanceType, result.nodeToPods)); err != nil {
		r.logger.Error("Failed to write winning result to file", "error", err)
	}
}

func (r *recommender) initializeSimulationState(simReq api.SimulationRequest) error {

	pods := util.ConstructPodsFromPodInfos(simReq.Pods, util.NilOr(simReq.PodOrder, common.SortDescending))
	nodes, err := util.ConstructNodesFromNodeInfos(simReq.Nodes, r.nodeTemplates)
	if err != nil {
		return err
	}
	r.state.unscheduledPods, r.state.scheduledPods = util.SplitScheduledAndUnscheduledPods(pods)
	r.state.originalUnscheduledPods = lo.SliceToMap[*corev1.Pod, string, *corev1.Pod](r.state.unscheduledPods, func(pod *corev1.Pod) (string, *corev1.Pod) {
		return pod.Name, pod
	})
	filteredNodePools := lo.Filter(simReq.NodePools, func(np api.NodePool, _ int) bool {
		return np.Current < np.Max
	})
	r.state.eligibleNodePools = lo.SliceToMap(filteredNodePools, func(np api.NodePool) (string, api.NodePool) {
		return np.Name, np
	})
	r.state.existingNodes = nodes
	r.state.priorityClasses = lo.Map(simReq.PriorityClasses, func(pt v1.PriorityClass, _ int) v1.PriorityClass {
		newPriorityClass := pt.DeepCopy()
		newPriorityClass.Namespace = common.DefaultNamespace
		return *newPriorityClass
	})
	return nil
}

func (r *recommender) computeTotalZonesAcrossNodePools() int {
	totalZones := 0
	for _, np := range r.state.eligibleNodePools {
		totalZones += np.Zones.Len()
	}
	return totalZones
}

func (r *recommender) runSimulation(ctx context.Context, runNum int) *runResult {
	var results []*runResult
	resultCh := make(chan *runResult, r.computeTotalZonesAcrossNodePools())
	r.triggerNodePoolSimulations(ctx, resultCh, runNum)

	// label, taint, result chan, error chan, close chan
	var errs error
	for result := range resultCh {
		slog.Info(fmt.Sprintf("%v\n", result.logs))
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
	printResultsSummary(runNum, results, winnerRunResult)
	return winnerRunResult
}

func (r *recommender) triggerNodePoolSimulations(ctx context.Context, resultCh chan *runResult, runNum int) {
	wg := &sync.WaitGroup{}
	r.logger.Info("Starting simulation runs for nodePools", "NodePools", maps.Keys(r.state.eligibleNodePools))

	for _, nodePool := range r.state.eligibleNodePools {
		wg.Add(1)
		runRef := lo.T2(simRunKey, rand.String(4))
		go r.runSimulationForNodePool(ctx, wg, nodePool, resultCh, runRef)
	}
	wg.Wait()
	close(resultCh)
}

func (r *recommender) runSimulationForNodePool(ctx context.Context, wg *sync.WaitGroup, nodePool api.NodePool, resultCh chan *runResult, runRef lo.Tuple2[string, string]) {
	var (
		scheduledPods []string
		err           error
	)
	defer wg.Done()
	defer func() {
		err = r.cleanUpNodePoolSimRun(ctx, runRef, &scheduledPods)
		if err != nil {
			slog.Error("Failed to clean up simulation run", "runRef", runRef.B, "error", err)
		}
	}()
	// create a copy of all nodes and scheduled pods only
	if scheduledPods, err = r.setupSimulationRun(ctx, runRef); err != nil {
		resultCh <- errorRunResult(err)
		return
	}
	for _, zone := range nodePool.Zones.UnsortedList() {
		runResult := r.runSimForZone(ctx, runRef, nodePool, zone)
		resultCh <- runResult
		if runResult.err != nil {
			return
		}
	}
}

func (r *recommender) runSimForZone(ctx context.Context, runRef lo.Tuple2[string, string], nodePool api.NodePool, zone string) *runResult {
	var (
		nodeName            string
		unscheduledPodNames []string
		simRunLogs          []string
	)
	simRunLogs = append(simRunLogs, fmt.Sprintf("Starting simulation run for nodePool: %s, zone: %s, runRef: %s...\n", nodePool.Name, zone, runRef.B))
	defer r.cleanUpSimRunForZone(ctx, nodePool.Name, runRef.B, &nodeName, &unscheduledPodNames)
	//foundNodeTemplate, ok := r.nodeTemplates[nodePool.InstanceType]
	foundNodeTemplate := util.FindNodeTemplate(r.nodeTemplates, nodePool.Name, zone)
	if foundNodeTemplate == nil {
		return errorRunResult(fmt.Errorf("node template not found for instance type %s", nodePool.InstanceType))
	}
	//if !ok {
	//	return errorRunResult(fmt.Errorf("node template not found for instance type %s", nodePool.InstanceType))
	//}
	node, err := util.ConstructNodeForSimRun(*foundNodeTemplate, nodePool.Name, zone, runRef)
	if err != nil {
		return errorRunResult(err)
	}
	nodeName = node.Name
	if err = kvcl.CreateAndUntaintNode(ctx, r.nc, common.NotReadyTaintKey, node); err != nil {
		return errorRunResult(err)
	}

	deployTime := time.Now()
	//r.logger.Info("Deploying unscheduled pods", "nodePool name", nodePool.Name, "runRef", runRef)
	unscheduledPods, err := r.createAndDeployUnscheduledPods(ctx, runRef)
	if err != nil {
		return errorRunResult(err)
	}
	//r.logger.Info("Deployed unscheduled pods", "nodePool name", nodePool.Name, "runRef", runRef)
	unscheduledPodNames = util.GetPodNames(unscheduledPods)
	scheduledPodNames, unSchedulePodNames, err := r.ec.GetPodSchedulingEvents(ctx, common.DefaultNamespace, deployTime, unscheduledPods, 10*time.Second)
	if err != nil {
		return errorRunResult(err)
	}
	simRunLogs = append(simRunLogs, fmt.Sprintf("Received Pod scheduling events for [nodePool: %s, runRef: %s]: scheduledPodNames: %v, unSchedulePodNames: %v\n", nodePool.Name, runRef.B, scheduledPodNames.UnsortedList(), unSchedulePodNames.UnsortedList()))
	simRunCandidatePods, err := r.pc.GetPodsMatchingPodNames(ctx, common.DefaultNamespace, scheduledPodNames.UnsortedList()...)
	if err != nil {
		return errorRunResult(err)
	}
	ns := r.scorer.Compute(node, simRunCandidatePods)
	simRunResult := r.computeRunResult(nodePool.Name, nodePool.InstanceType, zone, node, ns, getUpdatedPods(unscheduledPods, simRunCandidatePods))
	simRunLogs = append(simRunLogs, fmt.Sprintf("Simulation run result for [nodePool: %s, runRef: %s]: {score: %f, unscheduledPods: %v}\n", nodePool.Name, runRef.B, simRunResult.nodeScore, util.GetPodNames(simRunResult.unscheduledPods)))
	simRunResult.logs = simRunLogs
	return simRunResult
}

func (r *recommender) cleanUpSimRunForZone(ctx context.Context, nodePoolName, runRefVal string, nodeName *string, podNames *[]string) {
	r.logger.Info("cleaning up sim run", "nodePoolName", nodePoolName, "runRef", runRefVal)
	if nodeName != nil {
		//r.logger.Info("Deleting node", "nodePoolName", nodePoolName, "runRef", runRefVal, "nodeName", *nodeName)
		if err := r.nc.DeleteNodes(ctx, *nodeName); err != nil {
			r.logger.Error("Failed to delete node", "nodePoolName", nodePoolName, "runRef", runRefVal, "nodeName", *nodeName, "error", err)
		}
	}

	if podNames != nil && len(*podNames) > 0 {
		//r.logger.Info("Deleting pods", "nodePoolName", nodePoolName, "runRef", runRefVal, "podNames", *podNames)
		if err := r.pc.DeletePodsMatchingNames(ctx, common.DefaultNamespace, *podNames...); err != nil {
			r.logger.Error("Failed to delete pods", "nodePoolName", nodePoolName, "runRef", runRefVal, "podNames", *podNames, "error", err)
		}
	}
}

func getUpdatedPods(original []*corev1.Pod, scheduled []*corev1.Pod) []*corev1.Pod {
	updatedPods := make([]*corev1.Pod, 0, len(original))
	for _, op := range original {
		matchingScheduledPod, found := lo.Find(scheduled, func(p *corev1.Pod) bool {
			return p.Name == op.Name
		})
		if found {
			updatedPods = append(updatedPods, matchingScheduledPod)
		} else {
			updatedPods = append(updatedPods, op)
		}
	}
	return updatedPods
}

func (r *recommender) cleanUpNodePoolSimRun(ctx context.Context, runRef lo.Tuple2[string, string], podNames *[]string) error {
	labels := util.AsMap(runRef)
	var errs error
	errs = errors.Join(errs, r.pc.DeletePodsMatchingNames(ctx, common.DefaultNamespace, *podNames...))
	errs = errors.Join(errs, r.nc.DeleteNodesMatchingLabels(ctx, labels))
	return errs
}

func (r *recommender) setupSimulationRun(ctx context.Context, runRef lo.Tuple2[string, string]) ([]string, error) {
	// create copy of all nodes (barring existing nodes)
	clonedNodes := make([]*corev1.Node, 0, len(r.state.existingNodes))
	for _, node := range r.state.existingNodes {
		nodeCopy := node.DeepCopy()
		nodeCopy.Name = fromOriginalResourceName(nodeCopy.Name, runRef.B)
		nodeCopy.Labels[runRef.A] = runRef.B
		nodeCopy.Labels["kubernetes.io/hostname"] = nodeCopy.Name
		nodeCopy.ObjectMeta.UID = ""
		nodeCopy.ObjectMeta.ResourceVersion = ""
		nodeCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
		nodeCopy.Spec.Taints = append(nodeCopy.Spec.Taints, corev1.Taint{
			Key: runRef.A, Value: runRef.B, Effect: corev1.TaintEffectNoSchedule,
		})
		clonedNodes = append(clonedNodes, nodeCopy)
	}
	if err := kvcl.CreateAndUntaintNode(ctx, r.nc, common.NotReadyTaintKey, clonedNodes...); err != nil {
		return nil, err
	}

	// create copy of all scheduled pods
	clonedScheduledPods := make([]*corev1.Pod, 0, len(r.state.scheduledPods))
	for _, pod := range r.state.scheduledPods {
		podCopy := pod.DeepCopy()
		podCopy.Name = fromOriginalResourceName(podCopy.Name, runRef.B)
		if podCopy.Labels == nil {
			podCopy.Labels = make(map[string]string)
		}
		podCopy.Labels[runRef.A] = runRef.B
		podCopy.ObjectMeta.UID = ""
		podCopy.ObjectMeta.ResourceVersion = ""
		podCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
		podCopy.Spec.Tolerations = append(podCopy.Spec.Tolerations, corev1.Toleration{
			Key: runRef.A, Value: runRef.B, Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpEqual,
		})
		if len(podCopy.Spec.TopologySpreadConstraints) > 0 {
			updatedTSC := make([]corev1.TopologySpreadConstraint, 0, len(podCopy.Spec.TopologySpreadConstraints))
			for _, tsc := range podCopy.Spec.TopologySpreadConstraints {
				tsc.LabelSelector.MatchLabels[runRef.A] = runRef.B
				updatedTSC = append(updatedTSC, tsc)
			}
			podCopy.Spec.TopologySpreadConstraints = updatedTSC
		}
		podCopy.Spec.NodeName = fromOriginalResourceName(podCopy.Spec.NodeName, runRef.B)
		clonedScheduledPods = append(clonedScheduledPods, podCopy)
	}
	if err := r.pc.CreatePods(ctx, clonedScheduledPods...); err != nil {
		return nil, err
	}
	return util.GetPodNames(clonedScheduledPods), nil
}

func (r *recommender) resetNodePoolSimRun(ctx context.Context, nodeName string, runRef lo.Tuple2[string, string]) error {
	// remove the node with the nodeName
	if err := r.nc.DeleteNodes(ctx, nodeName); err != nil {
		return err
	}
	return r.pc.DeletePodsMatchingLabels(ctx, common.DefaultNamespace, util.AsMap(runRef))
}

func (r *recommender) createAndDeployUnscheduledPods(ctx context.Context, runRef lo.Tuple2[string, string]) ([]*corev1.Pod, error) {
	unscheduledPods := make([]*corev1.Pod, 0, len(r.state.unscheduledPods))
	for _, pod := range r.state.unscheduledPods {
		podCopy := pod.DeepCopy()
		podCopy.Name = fromOriginalResourceName(podCopy.Name, runRef.B)
		if podCopy.Labels == nil {
			podCopy.Labels = make(map[string]string)
		}
		podCopy.Labels[runRef.A] = runRef.B
		podCopy.ObjectMeta.UID = ""
		podCopy.ObjectMeta.ResourceVersion = ""
		podCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
		podCopy.Spec.Tolerations = []corev1.Toleration{
			{Key: runRef.A, Value: runRef.B, Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpEqual},
		}
		if len(podCopy.Spec.TopologySpreadConstraints) > 0 {
			updatedTSC := make([]corev1.TopologySpreadConstraint, 0, len(podCopy.Spec.TopologySpreadConstraints))
			for _, tsc := range podCopy.Spec.TopologySpreadConstraints {
				tsc.LabelSelector.MatchLabels[runRef.A] = runRef.B
				updatedTSC = append(updatedTSC, tsc)
			}
			podCopy.Spec.TopologySpreadConstraints = updatedTSC
		}
		podCopy.Spec.SchedulerName = common.BinPackingSchedulerName
		unscheduledPods = append(unscheduledPods, podCopy)
	}
	return unscheduledPods, r.pc.CreatePods(ctx, unscheduledPods...)
}

func (r *recommender) computeRunResult(nodePoolName, instanceType, zone string, node *corev1.Node, nodeScore float64, pods []*corev1.Pod) *runResult {
	if nodeScore == 0.0 {
		return &runResult{
			unscheduledPods: pods,
		}
	}
	unscheduledPods := make([]*corev1.Pod, 0, len(pods))
	nodeToPods := make(map[string][]podResourceInfo)
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			unscheduledPods = append(unscheduledPods, pod)
		} else {
			podResInfo := podResourceInfo{
				name:    pod.Name,
				request: cumulatePodRequests(pod),
			}
			nodeToPods[pod.Spec.NodeName] = append(nodeToPods[pod.Spec.NodeName], podResInfo)
		}
	}
	return &runResult{
		nodePoolName:    nodePoolName,
		nodeName:        toOriginalResourceName(node.Name),
		zone:            zone,
		instanceType:    instanceType,
		nodeScore:       nodeScore,
		unscheduledPods: unscheduledPods,
		nodeToPods:      nodeToPods,
		nodeCapacity:    node.Status.Capacity,
	}
}

func cumulatePodRequests(pod *corev1.Pod) corev1.ResourceList {
	sumRequests := make(corev1.ResourceList)
	for _, container := range pod.Spec.Containers {
		for name, quantity := range container.Resources.Requests {
			sumQuantity, ok := sumRequests[name]
			if ok {
				sumQuantity.Add(quantity)
				sumRequests[name] = sumQuantity
			} else {
				sumRequests[name] = quantity
			}
		}
	}
	return sumRequests
}

func (r *recommender) syncWinningResult(ctx context.Context, recommendation *api.ScaleUpRecommendation, winningRunResult *runResult) error {
	startTime := time.Now()
	defer func() {
		r.logger.Info("syncWinningResult for nodePool completed", "nodePool", recommendation.NodePoolName, "duration", time.Since(startTime).Seconds())
	}()
	scheduledPodNames, err := r.syncVirtualClusterWithWinningResult(ctx, winningRunResult)
	if err != nil {
		return err
	}
	return r.syncRecommenderStateWithWinningResult(ctx, recommendation, winningRunResult.nodeName, scheduledPodNames)
}

func (r *recommender) syncRecommenderStateWithWinningResult(ctx context.Context, recommendation *api.ScaleUpRecommendation, winningNodeName string, scheduledPodNames []string) error {
	winnerNode, err := r.nc.GetNode(ctx, types.NamespacedName{Name: winningNodeName, Namespace: common.DefaultNamespace})
	if err != nil {
		return err
	}
	r.state.existingNodes = append(r.state.existingNodes, winnerNode)
	scheduledPods, err := r.pc.GetPodsMatchingPodNames(ctx, common.DefaultNamespace, scheduledPodNames...)
	if err != nil {
		return err
	}
	for _, pod := range scheduledPods {
		r.state.scheduledPods = append(r.state.scheduledPods, pod)
		r.state.unscheduledPods = slices.DeleteFunc(r.state.unscheduledPods, func(p *corev1.Pod) bool {
			return p.Name == pod.Name
		})
	}
	r.state.updateEligibleNodePools(recommendation)
	return nil
}

func (r *recommender) syncVirtualClusterWithWinningResult(ctx context.Context, winningRunResult *runResult) ([]string, error) {
	nodeTemplate := util.FindNodeTemplate(r.nodeTemplates, winningRunResult.nodePoolName, winningRunResult.zone)
	if nodeTemplate == nil {
		return nil, fmt.Errorf("node template not found for instance type %s", winningRunResult.instanceType)
	}
	node, err := util.ConstructNodeFromNodeTemplate(*nodeTemplate, winningRunResult.nodePoolName, winningRunResult.zone)
	if err != nil {
		return nil, err
	}
	node.Name = winningRunResult.nodeName
	var originalPods []*corev1.Pod
	var scheduledPods []*corev1.Pod
	for nodeName, simPodResInfos := range winningRunResult.nodeToPods {
		for _, simPodResInfo := range simPodResInfos {
			podName := toOriginalResourceName(simPodResInfo.name)
			pod, ok := r.state.originalUnscheduledPods[podName]
			if !ok {
				return nil, fmt.Errorf("unexpected error, pod: %s not found in the original pods collection", podName)
			}
			originalPods = append(originalPods, pod)
			podCopy := pod.DeepCopy()
			podCopy.Spec.NodeName = toOriginalResourceName(nodeName)
			podCopy.ObjectMeta.ResourceVersion = ""
			podCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
			scheduledPods = append(scheduledPods, podCopy)
		}
	}
	if err = r.pc.CreatePods(ctx, scheduledPods...); err != nil {
		return nil, err
	}
	if err = kvcl.CreateAndUntaintNode(ctx, r.nc, common.NotReadyTaintKey, node); err != nil {
		return nil, err
	}
	return util.GetPodNames(scheduledPods), nil
}

func (r *recommender) initializeVirtualCluster(ctx context.Context) error {
	if r.state.existingNodes != nil {
		if err := util.CreateAndUntaintNodes(ctx, r.client, r.state.existingNodes); err != nil {
			return fmt.Errorf("failed to initialize virtual cluster with existing nodes: %w", err)
		}
	}
	for _, pc := range r.state.priorityClasses {
		if err := r.client.Create(ctx, &pc); err != nil {
			return fmt.Errorf("failed to initialize virtual cluster with priority class: %w", err)
		}
	}
	if r.state.scheduledPods != nil {
		if err := r.pc.CreatePods(ctx, r.state.scheduledPods...); err != nil {
			return fmt.Errorf("failed to initialize virtual cluster with scheduled pods: %w", err)
		}
	}
	return nil
}
