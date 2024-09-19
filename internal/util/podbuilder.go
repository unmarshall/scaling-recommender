package util

import (
	"fmt"
	"github.com/samber/lo"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultNamespace = "default"
)

type PodBuilder struct {
	objectMeta        metav1.ObjectMeta
	schedulerName     string
	spec              corev1.PodSpec
	nominatedNodeName string
	count             int
}

func NewPodBuilder() *PodBuilder {
	return &PodBuilder{
		objectMeta: metav1.ObjectMeta{
			Namespace: defaultNamespace,
			Labels:    make(map[string]string),
		},
	}
}

func (p *PodBuilder) Name(name string) *PodBuilder {
	p.objectMeta.Name = strings.ToLower(name)
	return p
}

func (p *PodBuilder) Labels(labels map[string]string) *PodBuilder {
	p.objectMeta.Labels = labels
	return p
}

func (p *PodBuilder) AddLabel(key string, value string) *PodBuilder {
	p.objectMeta.Labels[key] = value
	return p
}

func (p *PodBuilder) SchedulerName(schedulerName string) *PodBuilder {
	p.schedulerName = schedulerName
	return p
}

func (p *PodBuilder) Spec(spec corev1.PodSpec) *PodBuilder {
	p.spec = spec
	return p
}

func (p *PodBuilder) NominatedNodeName(nominatedNodeName string) *PodBuilder {
	p.nominatedNodeName = nominatedNodeName
	return p
}

func (p *PodBuilder) Count(count int) *PodBuilder {
	p.count = count
	return p
}

func (p *PodBuilder) Build() []*corev1.Pod {
	pods := make([]*corev1.Pod, 0, p.count)

	for i := 0; i < p.count; i++ {
		pod := &corev1.Pod{
			ObjectMeta: p.objectMeta,
			Spec:       p.spec,
		}
		pod.Name = fmt.Sprintf("%s-%d", p.objectMeta.Name, i)
		if !lo.IsEmpty(p.nominatedNodeName) {
			pod.Status.NominatedNodeName = p.nominatedNodeName
		}
		if !lo.IsEmpty(p.schedulerName) {
			pod.Spec.SchedulerName = p.schedulerName
		}
		pods = append(pods, pod)
	}
	return pods
}
