package factory

import (
	"log/slog"
	"unmarshall/scaling-recommender/internal/garden"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/scaler/scaleup"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

type factory struct {
	algos map[scaler.AlgoVariant]scaler.Recommender
}

func New(ga garden.Access, vcp virtualenv.ControlPlane, logger *slog.Logger) scaler.RecommenderFactory {
	algos := make(map[scaler.AlgoVariant]scaler.Recommender)
	// Register all scaling algorithms
	algos[scaler.MultiDimensionScoringScaleUpAlgo] = scaleup.NewRecommender(vcp, ga.GetAllReferenceNodes(), logger)
	//algos[DescendingCostScaleDownAlgo] = scaledown.NewDescendingCostRecommender(vcp.NodeControl(), vcp.PodControl(), vcp.EventControl(), pricingAccess)
	return &factory{
		algos: algos,
	}
}

func (f *factory) GetRecommender(variant scaler.AlgoVariant) scaler.Recommender {
	return f.algos[variant]
}
