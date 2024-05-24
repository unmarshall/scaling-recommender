package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/client/util"
)

func main() {
	//simRequest, err := util.CreateSimRequest(filepath.Join("client", "assets", "s1.json"))
	//simRequest, err := util.CreateSimRequest(filepath.Join("client", "assets", "s2.json"))
	//simRequest, err := util.CreateSimRequest(filepath.Join("client", "assets", "s3.json"))
	//simRequest, err := util.CreateSimRequest(filepath.Join("client", "assets", "s4.json"))
	//simRequest, err := util.CreateSimRequest(filepath.Join("client", "assets", "s5.json"))
	//if err != nil {
	//	slog.Error("Error in creating simulation request", "error", err)
	//	os.Exit(1)
	//}
	scenarios, err := util.ReadScenarios(filepath.Join("client", "assets", "scenarios.json"))
	dieOnError(err)
	simRequests, err := util.CreateSimRequests(scenarios)
	dieOnError(err)
	for i, simRequest := range simRequests {
		recommendation, err := runSimulation(simRequest)
		if err != nil {
			fmt.Println(err)
			continue
		}
		caRecommendation := util.ExtractCAScaleUpRecommendation(scenarios[i].NodeGroups)
		prettyPrint(recommendation, "recommendation: ")
		prettyPrint(caRecommendation, "CA recommendation: ")
	}
}

func prettyPrint[T any](obj T, message string) {
	jsonBytes, err := json.MarshalIndent(obj, "", "\t")
	dieOnError(err)
	fmt.Printf("%s:%s\n", message, string(jsonBytes))
}

func dieOnError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func runSimulation(simRequest api.SimulationRequest) (*api.RecommendationResponse, error) {
	reqURL := "http://localhost:8080/simulation/"
	reqBytes, err := json.Marshal(simRequest)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	r := bytes.NewReader(reqBytes)
	request, err := http.NewRequest(http.MethodPost, reqURL, r)

	if err != nil {
		fmt.Printf("Error creating request: %v", err)
		os.Exit(1)
	}
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	response, err := client.Do(request)
	if err != nil {
		fmt.Printf("Error in executing request: %v", err)
		os.Exit(1)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode == http.StatusOK {
		readBytes, err := io.ReadAll(response.Body)
		if err != nil {
			fmt.Printf("Error in reading response: %v", err)
			os.Exit(1)
		}
		recommendationResponse := &api.RecommendationResponse{}
		if err = json.Unmarshal(readBytes, recommendationResponse); err != nil {
			fmt.Printf("Error in unmarshalling response: %v", err)
			os.Exit(1)
		}
		return recommendationResponse, nil
	}
	return nil, fmt.Errorf("failed simulation: %s, StatusCode: %d, Status:%s", simRequest.ID, response.StatusCode, response.Status)
}
