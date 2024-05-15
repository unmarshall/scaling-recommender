package simulation

import (
	"log/slog"
	"net/http"
	"os"

	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/simulation/web"
)

type Handler struct {
	engine Engine
}

func NewSimulationHandler(engine Engine) *Handler {
	return &Handler{
		engine: engine,
	}
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	defer func() {
		err := r.Body.Close()
		if err != nil {
			slog.Info("error closing request body", "error", err)
		}
	}()

	// first clean up the virtual cluster
	if err := h.engine.VirtualControlPlane().FactoryReset(r.Context()); err != nil {
		web.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	simRequest, err := web.ParseSimulationRequest(r.Body)
	if err != nil {
		web.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	baseLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger := baseLogger.With("id", simRequest.ID)
	logger.Info("received simulation request", "request", simRequest.ID)

	recommender := h.engine.RecommenderFactory().GetRecommender(scaler.MultiDimensionScoringScaleUpAlgo)
	result := recommender.Run(r.Context(), *simRequest, *logger)
	if result.IsError() {
		web.ErrorResponse(w, http.StatusInternalServerError, result.Err.Error())
		return
	}
	response := api.RecommendationResponse{
		Recommendation: result.Ok,
	}
	if err = web.WriteJSON(w, http.StatusOK, response); err != nil {
		web.ErrorResponse(w, http.StatusInternalServerError, err.Error())
	}
}
