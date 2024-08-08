package factory

import (
	"log/slog"

	kvclapi "github.com/unmarshall/kvcl/api"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/scaler/scaleup"
)

type factory struct {
	algos map[scaler.AlgoVariant]scaler.Recommender
}

func New(vcp kvclapi.ControlPlane, logger *slog.Logger) scaler.RecommenderFactory {
	algos := make(map[scaler.AlgoVariant]scaler.Recommender)
	// Register all scaling algorithms
	algos[scaler.DefaultScaleUpAlgo] = scaleup.NewRecommender(vcp, logger)
	return &factory{
		algos: algos,
	}
}

func (f *factory) GetRecommender(variant scaler.AlgoVariant) scaler.Recommender {
	return f.algos[variant]
}
