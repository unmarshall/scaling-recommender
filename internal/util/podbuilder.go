package util

import (
	"unmarshall/scaling-recommender/internal/common"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultNamespace      = "default"
	defaultContainerImage = "registry.k8s.io/pause:3.5"
	defaultContainerName  = "pause"
)

type PodBuilder struct {
	objectMeta                metav1.ObjectMeta
	schedulerName             string
	resourceRequests          corev1.ResourceList
	topologySpreadConstraints []corev1.TopologySpreadConstraint
	tolerations               []corev1.Toleration
	nodeName                  string
}

func NewPodBuilder() *PodBuilder {
	return &PodBuilder{
		resourceRequests: make(corev1.ResourceList),
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

func (p *PodBuilder) ResourceRequests(resourceRequests corev1.ResourceList) *PodBuilder {
	p.resourceRequests = resourceRequests
	return p
}

func (p *PodBuilder) RequestMemory(quantity string) *PodBuilder {
	p.resourceRequests[corev1.ResourceMemory] = resource.MustParse(quantity)
	return p
}

func (p *PodBuilder) RequestCPU(quantity string) *PodBuilder {
	p.resourceRequests[corev1.ResourceCPU] = resource.MustParse(quantity)
	return p
}

func (p *PodBuilder) TopologySpreadConstraints(tscs []corev1.TopologySpreadConstraint) *PodBuilder {
	p.topologySpreadConstraints = tscs
	return p
}

func (p *PodBuilder) AddTopologySpreadConstraint(constraint corev1.TopologySpreadConstraint) *PodBuilder {
	p.topologySpreadConstraints = append(p.topologySpreadConstraints, constraint)
	return p
}

func (p *PodBuilder) Tolerations(tolerations []corev1.Toleration) *PodBuilder {
	p.tolerations = tolerations
	return p
}

func (p *PodBuilder) AddToleration(toleration corev1.Toleration) *PodBuilder {
	p.tolerations = append(p.tolerations, toleration)
	return p
}

func (p *PodBuilder) ScheduledOn(nodeName string) *PodBuilder {
	p.nodeName = nodeName
	return p
}

func (p *PodBuilder) Build() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: p.objectMeta,
		Spec: corev1.PodSpec{
			SchedulerName: EmptyOr(p.schedulerName, common.DefaultSchedulerName),
			Containers: []corev1.Container{
				{
					Name:  defaultContainerName,
					Image: defaultContainerImage,
					Resources: corev1.ResourceRequirements{
						Requests: p.resourceRequests,
					},
				},
			},
			TopologySpreadConstraints: p.topologySpreadConstraints,
			Tolerations:               p.tolerations,
			NodeName:                  p.nodeName,
		},
	}
}
