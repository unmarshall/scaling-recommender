package pricing

import (
	"log/slog"
	"os"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/json"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/util"
)

type InstancePricingAccess interface {
	Get3YearReservedPricing(instanceType string) float64
	ComputeCostRatiosForInstanceTypes(nodePools []api.NodePool) map[string]float64
}

func NewInstancePricingAccess() (InstancePricingAccess, error) {
	pricingMap, err := loadInstancePricing()
	if err != nil {
		return nil, err
	}
	return &access{
		pricingMap: pricingMap,
	}, nil
}

type access struct {
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

func loadInstancePricing() (map[string]InstancePricing, error) {
	var allPricing AllInstancePricing
	filePath, err := util.GetAbsoluteConfigPath("internal", "pricing", "assets", "aws_pricing_eu-west-1.json")
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(*filePath)
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
