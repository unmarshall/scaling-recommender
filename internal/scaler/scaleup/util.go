package scaleup

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"golang.org/x/exp/rand"
	"k8s.io/api/core/v1"
	"unmarshall/scaling-recommender/api"
)

func computeUnscheduledRatio(candidatePods []v1.Pod) float64 {
	var totalAssignedPods int
	for _, pod := range candidatePods {
		if pod.Spec.NodeName != "" {
			totalAssignedPods++
		}
	}
	return float64(len(candidatePods)-totalAssignedPods) / float64(len(candidatePods))
}

func computeWasteRatio(node *v1.Node, candidatePods []v1.Pod) float64 {
	var (
		targetNodeAssignedPods []v1.Pod
		totalMemoryConsumed    int64
	)
	for _, pod := range candidatePods {
		if pod.Spec.NodeName == node.Name {
			targetNodeAssignedPods = append(targetNodeAssignedPods, pod)
			for _, container := range pod.Spec.Containers {
				containerMemReq, ok := container.Resources.Requests[v1.ResourceMemory]
				if ok {
					totalMemoryConsumed += containerMemReq.MilliValue()
				}
			}
			slog.Info("NodPodAssignment: ", "pod", pod.Name, "node", pod.Spec.NodeName, "memory", pod.Spec.Containers[0].Resources.Requests.Memory().MilliValue())
		}
	}
	/*
		cpuToMemRatio = totalCPU/totalMemory

		cpuWasteRatio + memWasteRatio + (unscheduledRatio * costRatio)
	*/

	totalMemoryCapacity := node.Status.Capacity.Memory().MilliValue()
	return float64(totalMemoryCapacity-totalMemoryConsumed) / float64(totalMemoryCapacity)
}

func getWinningRunResult(results []*runResult) *runResult {
	if len(results) == 0 {
		return nil
	}
	var winner *runResult
	minScore := math.MaxFloat64
	var winningRunResults []*runResult
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
	return winner
}

func printResultsSummary(runNumber int, results []*runResult, winningResult *runResult) {
	if winningResult == nil || len(results) == 0 {
		slog.Info("No winning result found")
		return
	}
	slog.Info("Result summary for simulation run", "runNumber", runNumber)
	slog.Info("-----------------------------------------------------------------------")
	for _, r := range results {
		slog.Info("run result", "nodePoolName", r.nodePoolName, "zone", r.zone, "instanceType", r.instanceType, "score", r.nodeScore)
	}
	slog.Info("----- Winning RunResult -----", "nodePoolName", winningResult.nodePoolName, "zone", winningResult.zone, "instanceType", winningResult.instanceType, "nodeToPods", winningResult.nodeToPods)
}

func createScaleUpRecommendationFromResult(result runResult) api.ScaleUpRecommendation {
	return api.ScaleUpRecommendation{
		Zone:         result.zone,
		NodePoolName: result.nodePoolName,
		IncrementBy:  int32(1),
		InstanceType: result.instanceType,
	}
}

func appendScaleUpRecommendation(recommendations []api.ScaleUpRecommendation, recommendation api.ScaleUpRecommendation) []api.ScaleUpRecommendation {
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
	return recommendations
}

func fromOriginalResourceName(name, suffix string) string {
	return fmt.Sprintf(resourceNameFormat, name, suffix)
}

func toOriginalResourceName(simResName string) string {
	return strings.Split(simResName, "-simrun-")[0]
}
