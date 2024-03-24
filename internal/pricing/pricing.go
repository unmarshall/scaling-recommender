package pricing

import (
	"errors"
	"log/slog"
	"os"
	"path"
	"runtime"

	"k8s.io/apimachinery/pkg/util/json"
)

type InstancePricingAccess interface {
	Get3YearReservedPricing(instanceType string) float64
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
	pricingMap map[string]instancePricing
}

func (a *access) Get3YearReservedPricing(instanceType string) float64 {
	price, ok := a.pricingMap[instanceType]
	if !ok {
		slog.Error("instance type not found in pricing map", "instanceType", instanceType)
		return 0
	}
	return price.EDPPrice.Reserved3Year
}

func loadInstancePricing() (map[string]instancePricing, error) {
	var allPricing allInstancePricing
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errors.New("no caller information")
	}
	dirName := path.Dir(filename)
	filePath := path.Join(dirName, "aws_pricing_eu-west-1.json")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(content, &allPricing); err != nil {
		return nil, err
	}

	pricingMap := make(map[string]instancePricing)
	for _, pricing := range allPricing.Results {
		pricingMap[pricing.InstanceType] = pricing
	}
	return pricingMap, nil
}
