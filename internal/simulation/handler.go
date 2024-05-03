package simulation

import (
	"log/slog"
	"net/http"

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
	slog.Info("received simulation request", "request", simRequest)

}
