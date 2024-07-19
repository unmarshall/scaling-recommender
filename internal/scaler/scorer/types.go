package scorer

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/sets"
)

type ScoringStrategy string

const (
	PureCostStrategy          ScoringStrategy = "purecost"
	CostCpuMemWastageStrategy ScoringStrategy = "costcpumemwastage"
)

var AllScoringStrategies = sets.New(
	string(CostCpuMemWastageStrategy),
	string(PureCostStrategy),
)

func ValidateScoringStrategy(strategy string) error {
	if strategy == "" {
		return fmt.Errorf("scoring strategy is required")
	}
	if !AllScoringStrategies.Has(strategy) {
		return fmt.Errorf("scoring strategy %q is not supported", strategy)
	}
	return nil
}
