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

// Cluster is a virtual cluster that is started in-memory.
// It comprises kube-api-server, etcd and kube-scheduler and will be used for
// running simulations and making scaling recommendations.
type Cluster interface {
	// Start starts an in-memory cluster comprising:
	// 1. kube-api-server and etcd taking the binary from the vClusterBinaryAssetsPath.
	// 2. kube-scheduler.
	Start(ctx context.Context) error
	// Stop will stop the in-memory cluster.
	Stop() error
}

// NewCluster creates a new virtual cluster. None of the components of the
// virtual cluster are initialized and started. Call Start to initialize and start the virtual cluster.
func NewCluster(vClusterBinaryAssetsPath string) Cluster {
	return &cluster{
		binaryAssetsPath: vClusterBinaryAssetsPath,
	}
}

type cluster struct {
	// binaryAssetsPath is the path to the kube-api-server and etcd binaries.
	binaryAssetsPath string
	// restConfig is the rest config to connect to the in-memory kube-api-server.
	restConfig *rest.Config
	// client connects to the in-memory kube-api-server.
	client client.Client
	// testEnvironment starts kube-api-server and etcd processes in-memory.
	testEnvironment *envtest.Environment
	// scheduler is the Kubernetes scheduler run in-memory.
	scheduler *scheduler.Scheduler
}

func (c *cluster) Start(ctx context.Context) error {
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
	slog.Info("Wrote Kubeconfig file to connect to virtual cluster", "path", kubeConfigPath)
	c.testEnvironment = vEnv
	c.restConfig = cfg
	c.client = k8sClient

	slog.Info("Starting in-memory kube-scheduler...")
	return c.startScheduler(ctx, c.restConfig)
}

func (c *cluster) startKAPIAndEtcd() (vEnv *envtest.Environment, cfg *rest.Config, k8sClient client.Client, err error) {
	vEnv = &envtest.Environment{
		Scheme:                   scheme.Scheme,
		Config:                   nil,
		BinaryAssetsDirectory:    c.binaryAssetsPath,
		AttachControlPlaneOutput: true,
	}
	cfg, err = vEnv.Start()
	if err != nil {
		err = fmt.Errorf("failed to start virtual cluster: %w", err)
		return
	}
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		err = fmt.Errorf("failed to create client for virtual cluster: %w", err)
		return
	}
	return
}

func (c *cluster) startScheduler(ctx context.Context, restConfig *rest.Config) error {
	slog.Info("setting up in-memory kube-scheduler...")
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
	slog.Info("starting in-memory kube-scheduler...")
	sac.EventBroadcaster.StartRecordingToSink(ctx.Done())
	defer sac.EventBroadcaster.Shutdown()
	startInformersAndWaitForSync(ctx, sac, s)
	go s.Run(ctx)
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

func (c *cluster) Stop() error {
	slog.Info("Stopping in-memory kube-api-server and etcd...")
	if err := c.testEnvironment.Stop(); err != nil {
		slog.Warn("failed to stop in-memory kube-api-server and etcd", "error", err)
	}

	return nil
}
