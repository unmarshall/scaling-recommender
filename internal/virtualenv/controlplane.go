package virtualenv

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	schedulerappconfig "k8s.io/kubernetes/cmd/kube-scheduler/app/config"
	"k8s.io/kubernetes/pkg/scheduler"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// ControlPlane represents an in-memory control plane with limited components.
// It comprises kube-api-server, etcd and kube-scheduler and will be used for
// running simulations and making scaling recommendations.
type ControlPlane interface {
	// Start starts an in-memory controlPlane comprising:
	// 1. kube-api-server and etcd taking the binary from the vClusterBinaryAssetsPath.
	// 2. kube-scheduler.
	Start(ctx context.Context) error
	// Stop will stop the in-memory controlPlane.
	Stop() error
	// FactoryReset will reset the in-memory controlPlane to its initial state.
	FactoryReset(ctx context.Context) error
	// NodeControl returns the NodeControl for the in-memory controlPlane. Should only be called after Start.
	NodeControl() NodeControl
	// PodControl returns the PodControl for the in-memory controlPlane. Should only be called after Start.
	PodControl() PodControl
	// EventControl returns the EventControl for the in-memory controlPlane. Should only be called after Start.
	EventControl() EventControl
}

// NewControlPlane creates a new control plane. None of the components of the
// control-plane are initialized and started. Call Start to initialize and start the control-plane.
func NewControlPlane(vClusterBinaryAssetsPath string) ControlPlane {
	return &controlPlane{
		binaryAssetsPath: vClusterBinaryAssetsPath,
	}
}

type controlPlane struct {
	// binaryAssetsPath is the path to the kube-api-server and etcd binaries.
	binaryAssetsPath string
	// restConfig is the rest config to connect to the in-memory kube-api-server.
	restConfig *rest.Config
	// client connects to the in-memory kube-api-server.
	client client.Client
	// testEnvironment starts kube-api-server and etcd processes in-memory.
	testEnvironment *envtest.Environment
	// scheduler is the Kubernetes scheduler run in-memory.
	scheduler    *scheduler.Scheduler
	nodeControl  NodeControl
	podControl   PodControl
	eventControl EventControl
}

func (c *controlPlane) Start(ctx context.Context) error {
	slog.Info("Starting in-memory kube-api-server and etcd...")
	vEnv, cfg, k8sClient, err := c.startKAPIAndEtcd()
	if err != nil {
		return err
	}
	kubeConfigBytes, err := createKubeconfigFileForRestConfig(cfg)
	if err != nil {
		return err
	}
	kubeConfigPath, err := writeKubeConfig(kubeConfigBytes)
	if err != nil {
		return err
	}
	slog.Info("Wrote Kubeconfig file to connect to virtual controlPlane", "path", kubeConfigPath)
	c.testEnvironment = vEnv
	c.restConfig = cfg
	c.client = k8sClient
	c.nodeControl = NewNodeControl(k8sClient)
	c.podControl = NewPodControl(k8sClient)
	c.eventControl = NewEventControl(k8sClient)
	slog.Info("Starting in-memory kube-scheduler...")
	return c.startScheduler(ctx, c.restConfig)
}

func (c *controlPlane) Stop() error {
	slog.Info("Stopping in-memory kube-api-server and etcd...")
	if err := c.testEnvironment.Stop(); err != nil {
		slog.Warn("failed to stop in-memory kube-api-server and etcd", "error", err)
	}
	// once the context passed to the scheduler gets cancelled, the scheduler will stop as well.
	// No need to stop the scheduler explicitly.
	return nil
}

func (c *controlPlane) FactoryReset(ctx context.Context) error {
	slog.Info("Removing all nodes...")
	if err := c.NodeControl().DeleteAllNodes(ctx); err != nil {
		return fmt.Errorf("failed to delete all nodes: %w", err)
	}
	slog.Info("Removing all pods...")
	if err := c.PodControl().DeleteAllPods(ctx); err != nil {
		return fmt.Errorf("failed to delete all pods: %w", err)
	}
	if err := c.EventControl().DeleteAllEvents(ctx); err != nil {
		return fmt.Errorf("failed to delete all events: %w", err)
	}
	slog.Info("In-memory controlPlane factory reset successfully")
	return nil
}

func (c *controlPlane) NodeControl() NodeControl {
	if c.client == nil {
		slog.Error("controlPlane not started, first start the control plane and then call NodeControl")
		panic("controlPlane not started")
	}
	return NewNodeControl(c.client)
}

func (c *controlPlane) PodControl() PodControl {
	if c.client == nil {
		slog.Error("controlPlane not started, first start the control plane and then call NodeControl")
		panic("controlPlane not started")
	}
	return NewPodControl(c.client)
}

func (c *controlPlane) EventControl() EventControl {
	if c.client == nil {
		slog.Error("controlPlane not started, first start the control plane and then call NodeControl")
		panic("controlPlane not started")
	}
	return NewEventControl(c.client)
}

func (c *controlPlane) startKAPIAndEtcd() (vEnv *envtest.Environment, cfg *rest.Config, k8sClient client.Client, err error) {
	vEnv = &envtest.Environment{
		Scheme:                   scheme.Scheme,
		Config:                   nil,
		BinaryAssetsDirectory:    c.binaryAssetsPath,
		AttachControlPlaneOutput: true,
	}
	cfg, err = vEnv.Start()
	if err != nil {
		err = fmt.Errorf("failed to start virtual controlPlane: %w", err)
		return
	}
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		err = fmt.Errorf("failed to create client for virtual controlPlane: %w", err)
		return
	}
	return
}

func (c *controlPlane) startScheduler(ctx context.Context, restConfig *rest.Config) error {
	slog.Info("creating in-memory kube-scheduler configuration...")
	sac, err := createSchedulerAppConfig(ctx, restConfig)
	if err != nil {
		return err
	}
	recorderFactory := func(name string) events.EventRecorder {
		return sac.EventBroadcaster.NewRecorder(name)
	}
	s, err := scheduler.New(ctx,
		sac.Client,
		sac.InformerFactory,
		sac.DynInformerFactory,
		recorderFactory,
		scheduler.WithComponentConfigVersion(sac.ComponentConfig.TypeMeta.APIVersion),
		scheduler.WithKubeConfig(sac.KubeConfig),
		scheduler.WithProfiles(sac.ComponentConfig.Profiles...),
		scheduler.WithPercentageOfNodesToScore(sac.ComponentConfig.PercentageOfNodesToScore),
	)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}
	c.scheduler = s
	sac.EventBroadcaster.StartRecordingToSink(ctx.Done())
	defer sac.EventBroadcaster.Shutdown()
	startInformersAndWaitForSync(ctx, sac, s)
	go s.Run(ctx)
	slog.Info("in-memory kube-scheduler started successfully")
	return nil
}

func startInformersAndWaitForSync(ctx context.Context, sac *schedulerappconfig.Config, s *scheduler.Scheduler) {
	slog.Info("starting kube-scheduler informers...")
	sac.InformerFactory.Start(ctx.Done())
	if sac.DynInformerFactory != nil {
		sac.DynInformerFactory.Start(ctx.Done())
	}
	slog.Info("waiting for kube-scheduler informers to sync...")
	sac.InformerFactory.WaitForCacheSync(ctx.Done())
	if sac.DynInformerFactory != nil {
		sac.DynInformerFactory.WaitForCacheSync(ctx.Done())
	}
	if err := s.WaitForHandlersSync(ctx); err != nil {
		slog.Error("waiting for kube-scheduler handlers to sync", "error", err)
	}
}

/*
	Request {
	nodePoolSpec: [] NodePool {
	  "zones": "zone-a",
	  "instanceType": "n1-standard-2",
	  "maxCount": 3
		}
  	}
	PodSpec: {
	 "memory": 1,
	 "count": 1,
	}
*/
