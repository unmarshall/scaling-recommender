package pricing

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/json"
	"unmarshall/scaling-recommender/api"
)

type InstancePricingAccess interface {
	Get3YearReservedPricing(instanceType string) float64
	ComputeCostRatiosForInstanceTypes(nodePools []api.NodePool) map[string]float64
	ComputeAllCostRatios() map[string]float64
}

func NewInstancePricingAccess(provider string) (InstancePricingAccess, error) {
	a := &access{provider: provider}
	if err := a.initializeProviderPricing(); err != nil {
		return nil, err
	}
	return a, nil
}

type access struct {
	provider   string
	pricingMap map[string]InstancePricing
}

func (a *access) Get3YearReservedPricing(instanceType string) float64 {
	price, ok := a.pricingMap[instanceType]
	if !ok {
		slog.Error("instance type not found in pricing map", "instanceType", instanceType)
		return 0
	}
	return float64(price.EDPPrice.Reserved3Year)
}

func (a *access) ComputeCostRatiosForInstanceTypes(nodePools []api.NodePool) map[string]float64 {
	instanceTypeCostRatios := make(map[string]float64)
	totalCost := lo.Reduce[api.NodePool, float64](nodePools, func(totalCost float64, np api.NodePool, _ int) float64 {
		return totalCost + a.Get3YearReservedPricing(np.InstanceType)
	}, 0.0)
	for _, np := range nodePools {
		price := a.Get3YearReservedPricing(np.InstanceType)
		instanceTypeCostRatios[np.InstanceType] = price / totalCost
	}
	return instanceTypeCostRatios
}

func (a *access) initializeProviderPricing() (err error) {
	switch a.provider {
	case "aws":
		a.pricingMap, err = loadInstancePricing(filepath.Join("internal", "pricing", "assets", "aws_pricing_eu-west-1.json"))
	case "gcp":
		a.pricingMap, err = loadInstancePricing(filepath.Join("internal", "pricing", "assets", "gcp_pricing_eu-west1.json"))
	default:
		err = fmt.Errorf("provider not supported: %s", a.provider)
	}
	return
}

func loadInstancePricing(pricingJsonPath string) (map[string]InstancePricing, error) {
	var allPricing AllInstancePricing
	content, err := os.ReadFile(pricingJsonPath)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(content, &allPricing); err != nil {
		return nil, err
	}

	pricingMap := make(map[string]InstancePricing)
	for _, pricing := range allPricing.Results {
		pricingMap[pricing.InstanceType] = pricing
	}
	return pricingMap, nil
}
