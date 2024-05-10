package util

import (
	"github.com/samber/lo"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/pricing"
)

func ComputeCostRatiosForInstanceTypes(pa pricing.InstancePricingAccess, nodePools []api.NodePool) map[string]float64 {
	instanceTypeCostRatios := make(map[string]float64)
	totalCost := lo.Reduce[api.NodePool, float64](nodePools, func(totalCost float64, np api.NodePool, _ int) float64 {
		return totalCost + pa.Get3YearReservedPricing(np.InstanceType)
	}, 0.0)
	for _, np := range nodePools {
		price := pa.Get3YearReservedPricing(np.InstanceType)
		instanceTypeCostRatios[np.InstanceType] = price / totalCost
	}
	return instanceTypeCostRatios
}
