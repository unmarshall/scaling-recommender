package api

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// NodePool represents a worker in gardener.
type NodePool struct {
	Name         string   `json:"name"`
	Zones        []string `json:"zones"`
	Max          int32    `json:"max"`
	Current      int32    `json:"current"`
	InstanceType string   `json:"instanceType"`
}

// PodInfo contains relevant information about a pod.
type PodInfo struct {
	NamePrefix                string                            `json:"namePrefix"`
	Labels                    map[string]string                 `json:"labels,omitempty"`
	ScheduledOn               *NodeReference                    `json:"scheduledOn,omitempty"`
	Requests                  corev1.ResourceList               `json:"requests"`
	Tolerations               []corev1.Toleration               `json:"tolerations,omitempty"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	Count                     int                               `json:"count"`
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
	ID        string     `json:"id"`
	NodePools []NodePool `json:"nodePools"`
	Pods      []PodInfo  `json:"pods"`
	Nodes     []NodeInfo `json:"nodes"`
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
	Recommendation Recommendation `json:"recommendation"`
	RunTime        time.Duration  `json:"runTime"`
	Error          string         `json:"error,omitempty"`
}
