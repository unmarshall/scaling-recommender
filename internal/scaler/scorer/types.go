package scorer

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

// ScoringStrategy defines the strategy used to score nodes.
type ScoringStrategy string

const (
	// CostOnlyStrategy is a scoring strategy that scores nodes based on cost only.
	CostOnlyStrategy ScoringStrategy = "cost-only"
	// CostCpuMemWastageStrategy is a scoring strategy that scores nodes based on cost, CPU and memory wastage.
	CostCpuMemWastageStrategy ScoringStrategy = "cost-cpu-mem-wastage"
)

var scoringStrategies = sets.New(string(CostCpuMemWastageStrategy), string(CostOnlyStrategy))

// IsScoringStrategySupported checks if the passed in scoring strategy is supported.
func IsScoringStrategySupported(strategy string) bool {
	return scoringStrategies.Has(strategy)
}
