package scaleup

import (
	"fmt"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"log/slog"
	"strings"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/scaler"
)

func getWinningRunResult(results []*runResult) *runResult {
	if len(results) == 0 {
		return nil
	}

	var maxScore float64
	var winningRunResults []*runResult
	for _, v := range results {
		if v.nodeScore > maxScore {
			maxScore = v.nodeScore
		}
	}

	for _, v := range results {
		if v.nodeScore == maxScore {
			winningRunResults = append(winningRunResults, v)
		}
	}
	if len(winningRunResults) == 1 {
		return winningRunResults[0]
	}

	return tieBreak(winningRunResults)
}

func tieBreak(candidates []*runResult) *runResult {
	return lo.MaxBy(candidates, func(r1 *runResult, r2 *runResult) bool {
		return computeTotalResourceUnits(r1.nodeCapacity) > computeTotalResourceUnits(r2.nodeCapacity)
	})
}

func computeTotalResourceUnits(nodeCapacity corev1.ResourceList) float64 {
	var totalResourceUnits float64
	totalResourceUnits += float64(nodeCapacity.Cpu().Value() * scaler.CPUResourceUnitMultiplier)
	totalResourceUnits += float64(nodeCapacity.Memory().Value() * scaler.MemResourceUnitMultiplier)
	return totalResourceUnits
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
