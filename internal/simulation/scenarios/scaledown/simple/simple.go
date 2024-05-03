package simple

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/simulation"
	"unmarshall/scaling-recommender/internal/simulation/scenarios/scaledown"
	"unmarshall/scaling-recommender/internal/simulation/web"
)

type simpleScenario struct {
	engine simulation.Engine
}

func New(engine simulation.Engine) simulation.Scenario {
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
	lwf := w.(scaler.LogWriterFlusher)
	web.Logf(lwf, "Commencing scenario: %s ...", s.Name())
	podRequests := scaledown.ParseQueryParams(r, 2, 10)
	if err := scaledown.SetupScaleDownScenario(r.Context(), s.engine.VirtualControlPlane(), s.engine.GardenAccess(), lwf, s.engine.TargetShootCoordinate(), podRequests); err != nil {
		web.InternalError(lwf, err)
		return
	}
	result := s.getRecommendation(r.Context(), lwf)
	if result.IsError() {
		web.InternalError(lwf, result.Err)
	}
	web.Logf(lwf, "Scenario %s completed successfully", s.Name())
	web.Logf(lwf, "Recommendation: %v", result.Ok)
}

func (s *simpleScenario) getRecommendation(ctx context.Context, w scaler.LogWriterFlusher) scaler.Result {
	startTime := time.Now()
	defer func() {
		runDuration := time.Since(startTime)
		web.Logf(w, "Descending cost scale down recommender took %f seconds", runDuration.Seconds())
	}()
	recommender, found := s.engine.RecommenderFactory().GetRecommender(scaler.DescendingCostScaleDownAlgo)
	if !found {
		web.InternalError(w, fmt.Errorf("scaling algo variant %s not found: %w", scaler.DescendingCostScaleDownAlgo, scaler.ErrScalingAlgoVariantNotRegistered))
	}
	return recommender.Run(ctx, w)
}
