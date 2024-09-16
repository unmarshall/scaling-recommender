package scaleup

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/scaler"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
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
		NodeNames:    []string{result.nodeName},
	}
}

func appendScaleUpRecommendation(recommendations []api.ScaleUpRecommendation, recommendation api.ScaleUpRecommendation) []api.ScaleUpRecommendation {
	var found bool
	for i, r := range recommendations {
		if r.NodePoolName == recommendation.NodePoolName && r.Zone == recommendation.Zone {
			r.IncrementBy += recommendation.IncrementBy
			r.NodeNames = append(r.NodeNames, recommendation.NodeNames...)
			found = true
			recommendations[i] = r
		}
	}
	if !found {
		recommendations = append(recommendations, recommendation)
	}
	return recommendations
}

func appendNodeUtilisationInfo(winningRunResult runResult, utilisationInfos map[string]nodeUtilisationInfo) map[string]nodeUtilisationInfo {
	info := nodeUtilisationInfo{
		Zone:         winningRunResult.zone,
		NodePoolName: winningRunResult.nodePoolName,
		Capacity:     winningRunResult.nodeCapacity,
	}
	utilisationInfos[winningRunResult.nodeName] = info
	for nodeName, podInfos := range winningRunResult.nodeToPods {
		nodeName = toOriginalResourceName(nodeName)
		resourcesConsumed := &corev1.ResourceList{}
		resourcesConsumed = lo.Reduce(podInfos, func(resConsumed *corev1.ResourceList, item podResourceInfo, index int) *corev1.ResourceList {
			for k, v := range item.request {
				if val, ok := (*resConsumed)[k]; ok {
					val.Add(v)
					(*resConsumed)[k] = val
				} else {
					(*resConsumed)[k] = v
				}
			}
			return resConsumed
		}, resourcesConsumed)
		utilInfo, ok := utilisationInfos[nodeName]
		if ok {
			utilInfo.Pods = append(utilInfo.Pods, lo.Map(podInfos, func(pri podResourceInfo, _ int) string {
				return pri.name
			})...)

			utilInfo.ResourcesConsumed = *resourcesConsumed
			utilisationInfos[nodeName] = utilInfo
		} else {
			utilisationInfos[nodeName] = nodeUtilisationInfo{
				Pods: lo.Map(podInfos, func(pri podResourceInfo, _ int) string {
					return pri.name
				}),
				ResourcesConsumed: *resourcesConsumed,
			}
		}
	}
	return utilisationInfos
}

func fromOriginalResourceName(name, suffix string) string {
	name = strings.TrimPrefix(name, "shoot--")
	return fmt.Sprintf(resourceNameFormat, name, suffix)
}

func toOriginalResourceName(simResName string) string {
	return strings.Split(simResName, "-sr-")[0]
}

func makeResultsLogDir() (string, error) {
	rootDir, err := getProjectRoot()
	if err != nil {
		return "", err
	}
	tmpDir := filepath.Join(rootDir, "tmp")
	if err = os.Mkdir(tmpDir, os.ModePerm); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	return tmpDir, nil
}

func getProjectRoot() (string, error) {
	path, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(path)), nil
}
