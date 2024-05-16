package util

import (
	"encoding/json"
	"log"
	"os"

	"unmarshall/scaling-recommender/api"
)

func CreateSimRequest(jsonPath string) (*api.SimulationRequest, error) {
	file, err := os.Open(jsonPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	simRequest := &api.SimulationRequest{}
	err = json.NewDecoder(file).Decode(simRequest)
	if err != nil {
		log.Fatal(err)
	}
	return simRequest, nil
}
