package common

const (
	BinPackingSchedulerName = "bin-packing-scheduler"
	DefaultSchedulerName    = "default-scheduler"
	DefaultNamespace        = "default"
	KubeSystemNamespace     = "kube-system"
)

const (
	SortDescending = "desc"
	SortAscending  = "asc"
)

const (
	InstanceTypeLabelKey  = "node.kubernetes.io/instance-type"
	NotReadyTaintKey      = "node.kubernetes.io/not-ready"
	TopologyZoneLabelKey  = "topology.kubernetes.io/zone"
	TopologyHostLabelKey  = "kubernetes.io/hostname"
	WorkerPoolLabelKey    = "worker.gardener.cloud/pool"
	WorkerGroupLabelKey   = "worker.garden.sapcloud.io/group"
	GKETopologyLabelKey   = "topology.gke.io/zone"
	AWSTopologyLabelKey   = "topology.ebs.csi.aws.com/zone"
	FailureDomainLabelKey = "failure-domain.beta.kubernetes.io/zone"
)
