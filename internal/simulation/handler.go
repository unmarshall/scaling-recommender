package simulation

import (
	"log/slog"
	"net/http"
	"os"

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
	simRequest, err := web.ParseSimulationRequest(r.Body)
	if err != nil {
		web.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	baseLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger := baseLogger.With("id", simRequest.ID)
	logger.Info("received simulation request", "request", simRequest)

	recommender := h.engine.RecommenderFactory().GetRecommender(scaler.MultiDimensionScoringScaleUpAlgo)
	result := recommender.Run(r.Context(), *simRequest, *logger)
	if result.IsError() {
		web.ErrorResponse(w, http.StatusInternalServerError, result.Err.Error())
		return
	}
	// TODO change this to properly serialize the recommendation
	web.WriteJSON(w, http.StatusOK, web.ResponseEnvelope{"recommendation": result.Ok})
}
