package common

import (
	corev1 "k8s.io/api/core/v1"
)

// PodFilter is a predicate that takes in a Pod and returns the predicate result as a boolean.
type PodFilter func(pod *corev1.Pod) bool

// NodeFilter is a predicate that takes in a Node and returns the predicate result as a boolean.
type NodeFilter func(node *corev1.Node) bool

// EventFilter is a predicate that takes in an Event and returns the predicate result as a boolean.
type EventFilter func(event *corev1.Event) bool
