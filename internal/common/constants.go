package common

const (
	BinPackingSchedulerName = "bin-packing-scheduler"
	DefaultSchedulerName    = "default-scheduler"
	DefaultNamespace        = "default"
)

const (
	SortDescending = "desc"
	SortAscending  = "asc"
)

const (
	InstanceTypeLabelKey = "node.kubernetes.io/instance-type"
	NotReadyTaintKey     = "node.kubernetes.io/not-ready"
	TopologyZoneLabelKey = "topology.kubernetes.io/zone"
	TopologyHostLabelKey = "kubernetes.io/hostname"
)
