package util

import (
	"encoding/json"
	scalehist "github.com/elankath/gardener-scaling-history"
	"log"
	"os"
)

//func CreateClusterSnapshot(scenarios []scalehist.Scenario) ([]gsc.ClusterSnapshot, error) {
//	return lo.Map(scenarios, func(scenario scalehist.Scenario, index int) gsc.ClusterSnapshot {
//		return gsc.ClusterSnapshot{
//			ID:          fmt.Sprintf("Scenario-%d", index),
//			AutoscalerConfig: gsc.AutoscalerConfig{NodeTemplates: map[string]gsc.NodeTemplate{
//
//			}}
//			WorkerPools: mapToWorkerPools(scenario.NodeGroups),
//			Pods:        mapToPodInfos(lo.Flatten([][]scalehist.PodInfo{scenario.ScheduledPods, scenario.UnscheduledPods})),
//			Nodes:       mapToNodeInfos(scenario.Nodes),
//		}
//	}), nil
//}

//func ExtractCAScaleUpRecommendation(nodeGrps []scalehist.NodeGroupInfo) []api.ScaleUpRecommendation {
//	scaledNodeGrps := lo.Filter(nodeGrps, func(nodeGrp scalehist.NodeGroupInfo, _ int) bool {
//		return nodeGrp.TargetSize-nodeGrp.CurrentSize > 0
//	})
//	return lo.Map(scaledNodeGrps, func(ng scalehist.NodeGroupInfo, _ int) api.ScaleUpRecommendation {
//		return api.ScaleUpRecommendation{
//			Zone:         ng.Zone,
//			NodePoolName: ng.PoolName,
//			IncrementBy:  int32(ng.TargetSize - ng.CurrentSize),
//			InstanceType: ng.MachineType,
//		}
//	})
//}

func ReadScenario(filePath string) (*scalehist.Scenario, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	scenario := &scalehist.Scenario{}
	err = json.NewDecoder(file).Decode(scenario)

	if err != nil {
		log.Fatal(err)
	}
	return scenario, nil
}

//func mapToWorkerPools(nodeGrps []scalehist.NodeGroupInfo) []gsc.WorkerPoolInfo {
//	workerPools := make([]gsc.WorkerPoolInfo, 0, len(nodeGrps))
//	poolNameToNodeGroups := lo.GroupBy(nodeGrps, func(ng scalehist.NodeGroupInfo) string {
//		return ng.PoolName
//	})
//
//	for poolName, ngs := range poolNameToNodeGroups {
//		zones := lo.Reduce(ngs, func(zones []string, ng scalehist.NodeGroupInfo, _ int) []string {
//			return append(zones, ng.Zone)
//		}, []string{})
//		workerPools = append(workerPools, gsc.WorkerPoolInfo{
//			SnapshotMeta: gsc.SnapshotMeta{
//				Name: poolName,
//			},
//			Maximum:     ngs[0].PoolMax,
//			Minimum:     ngs[0].PoolMin,
//			Zones:       zones,
//			MachineType: ngs[0].MachineType,
//		})
//	}
//	return workerPools
//}
//
//func mapToPodInfos(podInfos []scalehist.PodInfo) []gsc.PodInfo {
//	return lo.Map(podInfos, func(podInfo scalehist.PodInfo, _ int) gsc.PodInfo {
//		return gsc.PodInfo{
//			SnapshotMeta: gsc.SnapshotMeta{
//				Name:      podInfo.Name,
//				Namespace: podInfo.Namespace,
//			},
//			Labels:            podInfo.Labels,
//			Spec:              podInfo.Spec,
//			NominatedNodeName: podInfo.NominatedNodeName,
//		}
//	})
//}
//
//func mapToNodeInfos(nodeInfos []scalehist.NodeInfo) []gsc.NodeInfo {
//	return lo.Map(nodeInfos, func(nodeInfo scalehist.NodeInfo, _ int) gsc.NodeInfo {
//		return gsc.NodeInfo{
//			SnapshotMeta: gsc.SnapshotMeta{
//				Name: nodeInfo.Name,
//			},
//			Labels:             nodeInfo.Labels,
//			Taints:             nodeInfo.Taints,
//			Allocatable:        nodeInfo.Allocatable,
//			Capacity:           nodeInfo.Capacity,
//			AllocatableVolumes: nodeInfo.AllocatableVolumes,
//		}
//	})
//}
