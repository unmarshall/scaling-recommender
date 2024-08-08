package scaleup

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	"unmarshall/scaling-recommender/internal/scaler/scorer"
	"unmarshall/scaling-recommender/internal/scaler/scorer/leastcost"

	"golang.org/x/exp/rand"
	"unmarshall/scaling-recommender/api"
)

func getWinningRunResult(results []*runResult, scoringStrategy scorer.ScoringStrategy) *runResult {
	if len(results) == 0 {
		return nil
	}
	var winner *runResult
	minScore := math.MaxFloat64
	var winningRunResults []*runResult
	for _, v := range results {
		if v.nodeScore.CumulativeScore < minScore {
			winner = v
			minScore = v.nodeScore.CumulativeScore
		}
	}
	for _, v := range results {
		if v.nodeScore.CumulativeScore == minScore {
			winningRunResults = append(winningRunResults, v)
		}
	}
	if scoringStrategy == scorer.LeastCostStrategy {
		if len(winningRunResults) > 1 {
			maxNodeCapacity := 0.0
			for _, winResult := range winningRunResults {
				resourceUnits := 0.0
				resourceUnits = resourceUnits + (float64(winResult.node.Status.Allocatable.Memory().Value()) * leastcost.ResourceUnitsPerMemory) //TODO: verify whether mem is stored in GB
				resourceUnits = resourceUnits + (float64(winResult.node.Status.Allocatable.Cpu().Value()) * leastcost.ResourceUnitsPerCPU)       //TODO: verify whether cpu is stored in cores

				if resourceUnits > maxNodeCapacity {
					maxNodeCapacity = resourceUnits
					winner = winResult
				}
			}
		}

	} else {
		rand.Seed(uint64(time.Now().UnixNano()))
		winningIndex := rand.Intn(len(winningRunResults))
		winner = winningRunResults[winningIndex]
	}
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
