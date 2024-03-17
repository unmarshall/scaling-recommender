package garden

import (
	corev1 "k8s.io/api/core/v1"
)

type Access interface {
	// GetReferenceNode gets a reference Node object from the reference cluster
	GetReferenceNode(instanceType string) *corev1.Node
}

func NewAccess() Access {
	return &access{}
}

type access struct {
}

func (a *access) GetReferenceNode(instanceType string) *corev1.Node {
	return nil
}
