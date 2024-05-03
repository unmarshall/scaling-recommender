package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"unmarshall/scaling-recommender/api"
)

func ParseSimulationRequest(reqBody io.ReadCloser) (*api.SimulationRequest, error) {
	simRequest := &api.SimulationRequest{}
	err := json.NewDecoder(reqBody).Decode(simRequest)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError

		switch {
		case errors.As(err, &syntaxError):
			return nil, fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return nil, fmt.Errorf("body contains badly formed JSON")
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return nil, fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return nil, fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)
		case errors.Is(err, io.EOF):
			return nil, errors.New("body must not be empty")
		default:
			return nil, err
		}
	}
	return simRequest, nil
}

type ResponseEnvelope map[string]interface{}

func WriteJSON(w http.ResponseWriter, statusCode int, data ResponseEnvelope) error {
	jsonBytes, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}
	jsonBytes = append(jsonBytes, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err = w.Write(jsonBytes); err != nil {
		return err
	}
	return nil
}

func ErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	responseEnvelope := ResponseEnvelope{"error": message}
	if err := WriteJSON(w, statusCode, responseEnvelope); err != nil {
		slog.Error("error writing response", "error", err)
		http.Error(w, "error writing response", http.StatusInternalServerError)
	}
}
