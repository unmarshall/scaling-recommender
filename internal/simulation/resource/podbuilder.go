package resource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"unmarshall/scaling-recommender/internal/util"
)

const (
	defaultNamespace      = "default"
	defaultContainerImage = "registry.k8s.io/pause:3.5"
	defaultContainerName  = "pause"
	defaultSchedulerName  = "default-scheduler"
)

type PodBuilder struct {
	objectMeta                metav1.ObjectMeta
	schedulerName             string
	resourceRequest           corev1.ResourceList
	topologySpreadConstraints []corev1.TopologySpreadConstraint
	tolerations               []corev1.Toleration
}

func NewPodBuilder() *PodBuilder {
	return &PodBuilder{
		resourceRequest: make(corev1.ResourceList),
		objectMeta: metav1.ObjectMeta{
			Namespace: defaultNamespace,
			Labels:    make(map[string]string),
		},
	}
}

func (p *PodBuilder) Name(name string) *PodBuilder {
	p.objectMeta.Name = name
	p.objectMeta.GenerateName = ""
	return p
}

func (p *PodBuilder) GenerateName(generateName string) *PodBuilder {
	p.objectMeta.GenerateName = generateName
	p.objectMeta.Name = ""
	return p
}

func (p *PodBuilder) Namespace(namespace string) *PodBuilder {
	p.objectMeta.Namespace = namespace
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

func (p *PodBuilder) RequestMemory(quantity string) *PodBuilder {
	p.resourceRequest[corev1.ResourceMemory] = resource.MustParse(quantity)
	return p
}

func (p *PodBuilder) RequestCPU(quantity string) *PodBuilder {
	p.resourceRequest[corev1.ResourceCPU] = resource.MustParse(quantity)
	return p
}

func (p *PodBuilder) AddTopologySpreadConstraints(constraint corev1.TopologySpreadConstraint) *PodBuilder {
	p.topologySpreadConstraints = append(p.topologySpreadConstraints, constraint)
	return p
}

func (p *PodBuilder) AddTolerations(toleration corev1.Toleration) *PodBuilder {
	p.tolerations = append(p.tolerations, toleration)
	return p
}

func (p *PodBuilder) Build() (*corev1.Pod, error) {
	if p.resourceRequest == nil || len(p.resourceRequest) == 0 {
		return nil, fmt.Errorf("resource request must be set")
	}
	return &corev1.Pod{
		ObjectMeta: p.objectMeta,
		Spec: corev1.PodSpec{
			SchedulerName: util.EmptyOr(p.schedulerName, defaultSchedulerName),
			Containers: []corev1.Container{
				{
					Name:  defaultContainerName,
					Image: defaultContainerImage,
					Resources: corev1.ResourceRequirements{
						Requests: p.resourceRequest,
					},
				},
			},
			TopologySpreadConstraints: p.topologySpreadConstraints,
			Tolerations:               p.tolerations,
		},
	}, nil
}
