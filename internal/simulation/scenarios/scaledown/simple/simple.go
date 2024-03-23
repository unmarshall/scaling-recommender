package simple

import (
	"net/http"

	"unmarshall/scaling-recommender/internal/simulation"
	"unmarshall/scaling-recommender/internal/simulation/executor"
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

}
