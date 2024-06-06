package util

import (
	"encoding/json"
	"fmt"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/sets"
	"log"
	"os"

	"github.com/elankath/scalehist"
	"unmarshall/scaling-recommender/api"
)

// FAILED SCHEDULING STREAM = {FAILED-PA1, FAILED-PB2, FAILED-PA1, FAILED-PC3, FAILED-PD1....}
// SCHEDULED STREAM = {SCHEDULED-PA1, SCHEDULED-PB2, SCHEDULED-PC3, ....}
// SCALE-UP = {TRIGGER-NG1-PA1, TRIGGER-NG1-PB2, TRIGGER-NG2-PC3...}
// SCALE-DOWN = {..., SCALEDOWN-NG1}
// HOW TO END SCENARIOS
// 1. ALL PODS GOT SCHEDULED AFTER RANGE OF TRIGGER SCALE-UPS
// 2. STOP SCENARIO COALESCING AT THE FIRST SCALE-DOWN
// 3.

func CreateSimRequests(scenarios []scalehist.Scenario) ([]api.SimulationRequest, error) {
	return lo.Map(scenarios, func(scenario scalehist.Scenario, index int) api.SimulationRequest {
		return api.SimulationRequest{
			ID:        fmt.Sprintf("Scenario-%d", index),
			NodePools: mapToNodePools(scenario.NodeGroups),
			Pods:      mapToPodInfos(lo.Flatten([][]scalehist.PodInfo{scenario.ScheduledPods, scenario.UnscheduledPods})),
			Nodes:     mapToNodeInfos(scenario.Nodes),
		}
	}), nil
}

func ExtractCAScaleUpRecommendation(nodeGrps []scalehist.NodeGroupInfo) []api.ScaleUpRecommendation {
	scaledNodeGrps := lo.Filter(nodeGrps, func(nodeGrp scalehist.NodeGroupInfo, _ int) bool {
		return nodeGrp.TargetSize-nodeGrp.CurrentSize > 0
	})
	return lo.Map(scaledNodeGrps, func(ng scalehist.NodeGroupInfo, _ int) api.ScaleUpRecommendation {
		return api.ScaleUpRecommendation{
			Zone:         ng.Zone,
			NodePoolName: ng.PoolName,
			IncrementBy:  int32(ng.TargetSize - ng.CurrentSize),
			InstanceType: ng.MachineType,
		}
	})
}

func ReadScenarios(filePath string) ([]scalehist.Scenario, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	analysis := &scalehist.Analysis{}
	err = json.NewDecoder(file).Decode(analysis)
	if err != nil {
		log.Fatal(err)
	}
	return analysis.Scenarios, nil
}

func mapToNodePools(nodeGrps []scalehist.NodeGroupInfo) []api.NodePool {
	nodePools := make(map[string]api.NodePool)
	for _, nodeGrp := range nodeGrps {
		if np, ok := nodePools[nodeGrp.PoolName]; ok {
			np.Max += int32(nodeGrp.MaxSize)
			np.Current += int32(nodeGrp.CurrentSize)
			np.Zones.Insert(nodeGrp.Zone)
			nodePools[nodeGrp.PoolName] = np
		} else {
			nodePools[nodeGrp.PoolName] = api.NodePool{
				Name:         nodeGrp.PoolName,
				Zones:        sets.New(nodeGrp.Zone),
				Max:          int32(nodeGrp.MaxSize),
				Current:      int32(nodeGrp.CurrentSize),
				InstanceType: nodeGrp.MachineType,
			}
		}
	}
	return lo.Values(nodePools)
}

func mapToPodInfos(podInfos []scalehist.PodInfo) []api.PodInfo {
	return lo.Map(podInfos, func(podInfo scalehist.PodInfo, _ int) api.PodInfo {
		return api.PodInfo{
			Name:              podInfo.Name,
			Labels:            podInfo.Labels,
			Spec:              podInfo.Spec,
			NominatedNodeName: podInfo.NominatedNodeName,
			Count:             1,
		}
	})
}

func mapToNodeInfos(nodeInfos []scalehist.NodeInfo) []api.NodeInfo {
	return lo.Map(nodeInfos, func(nodeInfo scalehist.NodeInfo, _ int) api.NodeInfo {
		return api.NodeInfo{
			Name:        nodeInfo.Name,
			Labels:      nodeInfo.Labels,
			Taints:      nodeInfo.Taints,
			Allocatable: nodeInfo.Allocatable,
			Capacity:    nodeInfo.Capacity,
		}
	})
}
