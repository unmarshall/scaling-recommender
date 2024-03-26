package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// PodFilter is a predicate that takes in a Pod and returns the predicate result as a boolean.
type PodFilter func(pod *corev1.Pod) bool

// NodeFilter is a predicate that takes in a Node and returns the predicate result as a boolean.
type NodeFilter func(node *corev1.Node) bool

// EventFilter is a predicate that takes in an Event and returns the predicate result as a boolean.
type EventFilter func(event *corev1.Event) bool

// ShootCoordinates represents the coordinates of a shoot cluster. It can be used to represent both the shoot and seed.
type ShootCoordinates struct {
	Project string
	Name    string
}

func (sc ShootCoordinates) GetNamespace() string {
	if sc.Project == "garden" {
		return "garden"
	}
	return fmt.Sprintf("garden-%s", sc.Project)
}

type AppConfig struct {
	Garden           string
	ReferenceShoot   ShootCoordinates
	BinaryAssetsPath string
}
