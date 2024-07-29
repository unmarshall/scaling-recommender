package api

import (
	gsc "github.com/elankath/gardener-scaling-common"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
)

// Create two funcs :- 1. Convert NodeGrpInfos to NodePools.
// 2. Extract Recommendation from NodeGrpInfos
// For each pair of scenario-CArecommendation, we run the simulation, get the recommendation
// and print both. Also print which is better in terms of cost.
// If priority expander is used by CA, then highlight that the comparison cannot be made

// AppConfig is the application configuration.
type AppConfig struct {
	Provider                 string
	BinaryAssetsPath         string
	TargetKVCLKubeConfigPath string
	ScoringStrategy          string
}

// NodePool represents a worker in gardener.
type NodePool struct {
	Name         string           `json:"name"`
	Zones        sets.Set[string] `json:"zones"`
	Max          int32            `json:"max"`
	Current      int32            `json:"current"`
	InstanceType string           `json:"instanceType"`
}

// PodInfo contains relevant information about a pod.
type PodInfo struct {
	Name              string            `json:"name,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Spec              corev1.PodSpec    `json:"spec"`
	NominatedNodeName string            `json:"nominatedNodeName,omitempty"`
	Count             int               `json:"count"`
}

// NodeInfo contains relevant information about a node.
type NodeInfo struct {
	Name        string              `json:"name"`
	Labels      map[string]string   `json:"labels"`
	Taints      []corev1.Taint      `json:"taints,omitempty"`
	Allocatable corev1.ResourceList `json:"allocatable"`
	Capacity    corev1.ResourceList `json:"capacity"`
}

// NodeReference captures sufficient information to identify a node type on which a pod is scheduled.
type NodeReference struct {
	Name     string `json:"name"`
	PoolName string `json:"poolName"`
	Zone     string `json:"zone"`
}

// SimulationRequest is a request to simulate a scenario.
type SimulationRequest struct {
	ID              string                       `json:"id"`
	NodePools       []NodePool                   `json:"nodePools"`
	Pods            []PodInfo                    `json:"pods"`
	PriorityClasses []schedulingv1.PriorityClass `json:"priorityClasses"`
	Nodes           []NodeInfo                   `json:"nodes"`
	// NodeTemplates is a map keyed on the instance Type.
	NodeTemplates map[string]gsc.NodeTemplate `json:"nodeTemplates"`
	// PodOrder is the order in which pods will be sorted and scheduled.
	// If not provided, pods will be ordered in descending order of requested resources.
	PodOrder *string `json:"podOrder,omitempty"`
}

type Recommendation struct {
	ScaleUp   []ScaleUpRecommendation `json:"scaleUp,omitempty"`
	ScaleDown []string                `json:"scaleDown,omitempty"`
}

type ScaleUpRecommendation struct {
	Zone         string `json:"zone"`
	NodePoolName string `json:"nodePoolName"`
	IncrementBy  int32  `json:"incrementBy"`
	InstanceType string `json:"instanceType"`
}

type RecommendationResponse struct {
	Recommendation  Recommendation     `json:"recommendation"`
	UnscheduledPods []client.ObjectKey `json:"unscheduledPods"`
	RunTime         string             `json:"runTime"`
	Error           string             `json:"error,omitempty"`
}
