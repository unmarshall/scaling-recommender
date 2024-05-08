package api

import corev1 "k8s.io/api/core/v1"

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
	Name                      string                            `json:"name"`
	Labels                    map[string]string                 `json:"labels"`
	ScheduledOn               *NodeReference                    `json:"scheduledOn,omitempty"`
	Requests                  corev1.ResourceList               `json:"requests"`
	Tolerations               []corev1.Toleration               `json:"tolerations"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints"`
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
	// PodOrder is the order in which pods will be sorted and scheduled.
	// If not provided, pods will be ordered in descending order of requested resources.
	PodOrder *string
}
