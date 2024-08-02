package scaleup

import (
	"context"
	"errors"
	"fmt"
	v1 "k8s.io/api/scheduling/v1"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/samber/lo"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"unmarshall/scaling-recommender/internal/common"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/util"

	corev1 "k8s.io/api/core/v1"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/scaler"

	kvclapi "github.com/unmarshall/kvcl/api"
	kvcl "github.com/unmarshall/kvcl/pkg/control"
)

const (
	simRunKey          = "app.kubernetes.io/simulation-run"
	resourceNameFormat = "%s-simrun-%s"
)

type recommender struct {
	nc            kvclapi.NodeControl
	pc            kvclapi.PodControl
	ec            kvclapi.EventControl
	pa            pricing.InstancePricingAccess
	client        client.Client
	scorer        scaler.Scorer
	state         simulationState
	nodeTemplates map[string]api.NodeTemplate
	baseLogger    *slog.Logger
	logger        *slog.Logger
}

type runResult struct {
	nodePoolName    string
	nodeName        string
	zone            string
	instanceType    string
	nodeScore       scaler.NodeScore
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
		nc:         vcp.NodeControl(),
		pc:         vcp.PodControl(),
		ec:         vcp.EventControl(),
		client:     vcp.Client(),
		baseLogger: baseLogger,
	}
}

func (r *recommender) Run(ctx context.Context, scorer scaler.Scorer, simReq api.SimulationRequest) scaler.Result {
	var (
		recommendations []api.ScaleUpRecommendation
		runNumber       int
	)
	r.scorer = scorer
	r.logger = r.baseLogger.With("id", simReq.ID)
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
		r.logger.Info("For scale-up recommender", "runNumber", runNumber, "winning-score", recommendation)
		recommendations = appendScaleUpRecommendation(recommendations, recommendation)
	}
	return scaler.OkScaleUpResult(recommendations, r.state.getUnscheduledPodObjectKeys())
}

func (r *recommender) initializeSimulationState(simReq api.SimulationRequest) error {
	pods := util.ConstructPodsFromPodInfos(simReq.Pods, util.NilOr(simReq.PodOrder, common.SortDescending))
	nodes, err := util.ConstructNodesFromNodeInfos(simReq.Nodes)
	if err != nil {
		return err
	}
	r.state.unscheduledPods, r.state.scheduledPods = util.SplitScheduledAndUnscheduledPods(pods)
	r.state.originalUnscheduledPods = lo.SliceToMap[*corev1.Pod, string, *corev1.Pod](r.state.unscheduledPods, func(pod *corev1.Pod) (string, *corev1.Pod) {
		return pod.Name, pod
	})
	r.state.eligibleNodePools = lo.SliceToMap(simReq.NodePools, func(np api.NodePool) (string, api.NodePool) {
		return np.Name, np
	})
	r.state.existingNodes = nodes
	r.state.priorityClasses = simReq.PriorityClasses
	return nil
}

func (r *recommender) runSimulation(ctx context.Context, runNum int) *runResult {
	var results []*runResult
	resultCh := make(chan *runResult, len(r.state.eligibleNodePools))
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
	printResultsSummary(runNum, results, winnerRunResult)
	return winnerRunResult
}

func (r *recommender) triggerNodePoolSimulations(ctx context.Context, resultCh chan *runResult, runNum int) {
	wg := &sync.WaitGroup{}
	r.logger.Info("Starting simulation runs for nodePools", "NodePools", maps.Keys(r.state.eligibleNodePools))

	for _, nodePool := range r.state.eligibleNodePools {
		wg.Add(1)
		runRef := lo.T2(simRunKey, nodePool.Name+"-"+strconv.Itoa(runNum))
		go r.runSimulationForNodePool(ctx, wg, nodePool, resultCh, runRef)
	}
	wg.Wait()
	close(resultCh)
}

func (r *recommender) runSimulationForNodePool(ctx context.Context, wg *sync.WaitGroup, nodePool api.NodePool, resultCh chan *runResult, runRef lo.Tuple2[string, string]) {
	simRunStartTime := time.Now()
	defer wg.Done()
	defer func() {
		r.logger.Info("Simulation run completed", "runRef", runRef.B, "nodePool", nodePool.Name, "duration", time.Since(simRunStartTime).Seconds())
	}()
	defer func() {
		if err := r.cleanUpNodePoolSimRun(ctx, runRef); err != nil {
			// In the productive code, there will not be any real KAPI and ETCD. Fake API server will never return an error as everything will be in memory.
			// For now, we are only logging this error as in the POC code since the caller of recommender will re-initialize the virtual cluster.
			r.logger.Error("Error cleaning up simulation run", "runRef", runRef.B, "err", err)
		}
	}()
	var (
		node *corev1.Node
		err  error
	)
	// create a copy of all nodes and scheduled pods only
	if err = r.setupSimulationRun(ctx, runRef); err != nil {
		resultCh <- errorRunResult(err)
		return
	}
	for _, zone := range nodePool.Zones.UnsortedList() {
		if node != nil {
			if err = r.resetNodePoolSimRun(ctx, node.Name, runRef); err != nil {
				resultCh <- errorRunResult(err)
				return
			}
		}
		foundNodeTemplate, ok := r.nodeTemplates[nodePool.InstanceType]
		if !ok {
			resultCh <- errorRunResult(fmt.Errorf("node template not found for instance type %s", nodePool.InstanceType))
			return
		}
		node, err = util.ConstructNodeForSimRun(foundNodeTemplate, nodePool.Name, zone, runRef)
		if err != nil {
			resultCh <- errorRunResult(err)
			return
		}
		if err = kvcl.CreateAndUntaintNode(ctx, r.nc, common.NotReadyTaintKey, node); err != nil {
			resultCh <- errorRunResult(err)
			return
		}

		deployTime := time.Now()
		unscheduledPods, err := r.createAndDeployUnscheduledPods(ctx, runRef)
		if err != nil {
			resultCh <- errorRunResult(err)
			return
		}
		scheduledPodNames, unSchedulePodNames, err := r.ec.GetPodSchedulingEvents(ctx, common.DefaultNamespace, deployTime, unscheduledPods, 10*time.Second)
		if err != nil {
			resultCh <- errorRunResult(err)
			return
		}
		slog.Info("Received Pod scheduling events", "scheduledPodNames", scheduledPodNames.UnsortedList(), "unSchedulePodNames", unSchedulePodNames.UnsortedList())
		simRunCandidatePods, err := r.pc.GetPodsMatchingPodNames(ctx, common.DefaultNamespace, util.GetPodNames(unscheduledPods)...)
		if err != nil {
			resultCh <- errorRunResult(err)
			return
		}
		ns := r.scorer.Compute(node, simRunCandidatePods)
		resultCh <- r.computeRunResult(nodePool.Name, nodePool.InstanceType, zone, node.Name, ns, simRunCandidatePods)
	}
}

func (r *recommender) cleanUpNodePoolSimRun(ctx context.Context, runRef lo.Tuple2[string, string]) error {
	labels := util.AsMap(runRef)
	slog.Info("Cleaning up simulation run", "runRef", runRef.B)
	var errs error
	errs = errors.Join(errs, r.pc.DeletePodsMatchingLabels(ctx, common.DefaultNamespace, labels))
	errs = errors.Join(errs, r.nc.DeleteNodesMatchingLabels(ctx, labels))
	return errs
}

func (r *recommender) setupSimulationRun(ctx context.Context, runRef lo.Tuple2[string, string]) error {
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
		nodeCopy.Spec.Taints = []corev1.Taint{
			{Key: runRef.A, Value: runRef.B, Effect: corev1.TaintEffectNoSchedule},
		}
		clonedNodes = append(clonedNodes, nodeCopy)
	}
	if err := kvcl.CreateAndUntaintNode(ctx, r.nc, common.NotReadyTaintKey, clonedNodes...); err != nil {
		return err
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
		podCopy.Spec.NodeName = fromOriginalResourceName(podCopy.Spec.NodeName, runRef.B)
		clonedScheduledPods = append(clonedScheduledPods, podCopy)
	}
	if err := r.pc.CreatePods(ctx, clonedScheduledPods...); err != nil {
		return err
	}
	return nil
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

func (r *recommender) computeRunResult(nodePoolName, instanceType, zone, nodeName string, score scaler.NodeScore, pods []corev1.Pod) *runResult {
	if score.UnscheduledRatio == 1.0 {
		return &runResult{}
	}
	unscheduledPods := make([]corev1.Pod, 0, len(pods))
	nodeToPods := make(map[string][]types.NamespacedName)
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			unscheduledPods = append(unscheduledPods, pod)
		} else {
			nodeToPods[pod.Spec.NodeName] = append(nodeToPods[pod.Spec.NodeName], client.ObjectKeyFromObject(&pod))
		}
	}
	return &runResult{
		nodePoolName:    nodePoolName,
		nodeName:        toOriginalResourceName(nodeName),
		zone:            zone,
		instanceType:    instanceType,
		nodeScore:       score,
		unscheduledPods: unscheduledPods,
		nodeToPods:      nodeToPods,
	}
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
		r.state.scheduledPods = append(r.state.scheduledPods, &pod)
		r.state.unscheduledPods = slices.DeleteFunc(r.state.unscheduledPods, func(p *corev1.Pod) bool {
			return p.Name == pod.Name
		})
	}
	r.state.updateEligibleNodePools(recommendation)
	return nil
}

func (r *recommender) syncVirtualClusterWithWinningResult(ctx context.Context, winningRunResult *runResult) ([]string, error) {
	nodeTemplate, ok := r.nodeTemplates[winningRunResult.instanceType]
	if !ok {
		return nil, fmt.Errorf("node template not found for instance type %s", winningRunResult.instanceType)
	}
	node, err := util.ConstructNodeFromNodeTemplate(nodeTemplate, winningRunResult.nodePoolName, winningRunResult.zone)
	if err != nil {
		return nil, err
	}
	node.Name = winningRunResult.nodeName
	var originalPods []*corev1.Pod
	var scheduledPods []*corev1.Pod
	for nodeName, simPodObjectKeys := range winningRunResult.nodeToPods {
		for _, simPodObjectKey := range simPodObjectKeys {
			podName := toOriginalResourceName(simPodObjectKey.Name)
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
		if err := r.nc.CreateNodes(ctx, r.state.existingNodes...); err != nil {
			return fmt.Errorf("failed to initialize virtual cluster with existing nodes: %w", err)
		}
	}
	if r.state.scheduledPods != nil {
		if err := r.pc.CreatePods(ctx, r.state.scheduledPods...); err != nil {
			return fmt.Errorf("failed to initialize virtual cluster with scheduled pods: %w", err)
		}
	}
	for _, pc := range r.state.priorityClasses {
		if err := r.client.Create(ctx, &pc); err != nil {
			return fmt.Errorf("failed to initialize virtual cluster with priority class: %w", err)
		}
	}
	return nil
}
