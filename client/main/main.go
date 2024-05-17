package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	//simRequest, err := util.CreateSimRequest(filepath.Join("client", "assets", "s4a.json"))
	simRequest, err := util.CreateSimRequest(filepath.Join("client", "assets", "s5.json"))
	if err != nil {
		slog.Error("Error in creating simulation request", "error", err)
		os.Exit(1)
	}

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
		Timeout: 60 * time.Minute,
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
		var recommendationResponse api.RecommendationResponse
		if err = json.Unmarshal(readBytes, &recommendationResponse); err != nil {
			fmt.Printf("Error in unmarshalling response: %v", err)
			os.Exit(1)
		}
		respJson, err := json.MarshalIndent(recommendationResponse, "", "\t")
		if err != nil {
			fmt.Printf("Error in marshalling response: %v", err)
			os.Exit(1)
		}
		fmt.Printf("Response status code: %d\n Recommendation response: %+v", response.StatusCode, string(respJson))
	}

}

//simReq := api.SimulationRequest{
//	ID: "s4-test",
//	NodePools: []api.NodePool{
//		{Name: "p1", InstanceType: "m5.large", Max: 12, Zones: []string{"eu-west-1a"}},
//		{Name: "p2", InstanceType: "m5.xlarge", Max: 5, Zones: []string{"eu-west-1b"}},
//		{Name: "p3", InstanceType: "m5.4xlarge", Max: 5, Zones: []string{"eu-west-1c"}},
//	},
//	Pods: []api.PodInfo{
//		{
//			NamePrefix: "poda-",
//			Requests:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("5Gi"), corev1.ResourceCPU: resource.MustParse("100m")},
//			Labels:     map[string]string{"variant": "small"},
//			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
//				{
//					MaxSkew:     1,
//					TopologyKey: "topology.kubernetes.io/zone",
//					LabelSelector: &metav1.LabelSelector{
//						MatchLabels: map[string]string{"variant": "small"},
//					},
//					MinDomains:        pointer.Int32(3),
//					WhenUnsatisfiable: corev1.DoNotSchedule,
//				},
//			},
//			Count: 10,
//		},
//		{
//			NamePrefix: "podb-",
//			Requests:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: resource.MustParse("12Gi"), corev1.ResourceCPU: resource.MustParse("200m")},
//			Labels:     map[string]string{"variant": "large"},
//			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
//				{
//					MaxSkew:     1,
//					TopologyKey: "kubernetes.io/hostname",
//					LabelSelector: &metav1.LabelSelector{
//						MatchLabels: map[string]string{"variant": "large"},
//					},
//					MinDomains:        pointer.Int32(3),
//					WhenUnsatisfiable: corev1.DoNotSchedule,
//				},
//			},
//			Count: 2,
//		},
//	},
//}
//marshal, err := json.Marshal(simReq)
//if err != nil {
//return
//}
//println(string(marshal))
