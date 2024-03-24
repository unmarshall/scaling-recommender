package simple

import (
	"fmt"
	"net/http"

	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/simulation"
	"unmarshall/scaling-recommender/internal/simulation/executor"
	"unmarshall/scaling-recommender/internal/simulation/web"
)

type simpleScenario struct {
	engine executor.Engine
}

func New(engine executor.Engine) simulation.Scenario {
	return &simpleScenario{
		engine: engine,
	}
}

func (s *simpleScenario) Name() string {
	return "simple"
}

func (s *simpleScenario) HandlerFn() simulation.HandlerFn {
	return s.run
}

func (s *simpleScenario) run(w http.ResponseWriter, r *http.Request) {
	web.Logf(w, "Commencing scenario: %s ...", s.Name())
	recommender, found := s.engine.RecommenderFactory().GetRecommender(scaler.DescendingCostScaleDownAlgo)
	if !found {
		web.InternalError(w, fmt.Errorf("scaling algo variant %s not found: %w", scaler.DescendingCostScaleDownAlgo, scaler.ErrScalingAlgoVariantNotRegistered))
	}

	recommender.Run(r.Context(), w.(scaler.LogWriterFlusher))

}
