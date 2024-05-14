package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"net/http"
	"os"
	"time"
	"unmarshall/scaling-recommender/api"
)

//(single zone workerpool)
//PodA : 5Gb -> 20 Replicas
//NG1 : m5.large -> 8Gb; NG1Max: 12
//NG2 : m5.4xlarge -> 64Gb; NG2Max: 4

func main() {
	simRequest := api.SimulationRequest{
		ID: "test",
		NodePools: []api.NodePool{
			{Name: "p1", InstanceType: "m5.large", Max: 12, Zones: []string{"eu-west-1a"}},
			{Name: "p2", InstanceType: "m5.4xlarge", Max: 4, Zones: []string{"eu-west-1a"}},
		},
		Pods: []api.PodInfo{
			{Name: "a1", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a2", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a3", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a4", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a5", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a6", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a7", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a8", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a9", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a10", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a11", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a12", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a13", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a14", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a15", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a16", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a17", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a18", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a19", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "a20", Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")}},
		},
	}

	reqURL := "https://localhost:8080/simulation/"
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
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		fmt.Printf("Error in executing request: %v", err)
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		readBytes, err := io.ReadAll(response.Body)
		if err != nil {
			fmt.Printf("Error in reading response: %v", err)
			os.Exit(1)
		}
		var recommendation *api.ScaleUpRecommendation
		if err = json.Unmarshal(readBytes, recommendation); err != nil {
			fmt.Printf("Error in unmarshalling response: %v", err)
			os.Exit(1)
		}
		fmt.Printf("Response status code: %d\n Recommendation: %+v", response.StatusCode, recommendation)
	}

}
