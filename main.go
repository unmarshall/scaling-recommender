package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"unmarshall/scaling-recommender/internal/app"
	"unmarshall/scaling-recommender/internal/garden"
	"unmarshall/scaling-recommender/internal/pricing"
	"unmarshall/scaling-recommender/internal/simulation"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

func main() {
	defer app.OnExit()
	utilruntime.Must(gardencorev1beta1.AddToScheme(scheme.Scheme))
	utilruntime.Must(seedmanagementv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(machinev1alpha1.AddToScheme(scheme.Scheme))
	ctx := setupSignalHandler()

	appConfig, err := parseCmdArgs()
	if err != nil {
		app.ExitAppWithError(1, fmt.Errorf("failed to parse command line arguments: %w", err))
	}

	slog.Info("starting scaling recommender environment", "appConfig", appConfig)
	gardenAccess := initializeGardenAccess(ctx, appConfig)
	vCluster := startVirtualCluster(ctx, appConfig)
	defer func() {
		slog.Info("shutting down virtual cluster...")
		if err = vCluster.Stop(); err != nil {
			slog.Error("failed to stop virtual cluster", "error", err)
		}
	}()
	pricingAccess, err := pricing.NewInstancePricingAccess()
	if err != nil {
		app.ExitAppWithError(1, fmt.Errorf("failed to create instance pricing access: %w", err))
	}
	startScenarioExecutorEngine(gardenAccess, vCluster, pricingAccess, appConfig.TargetShoot)
	<-ctx.Done()
}

func startVirtualCluster(ctx context.Context, appConfig app.Config) virtualenv.ControlPlane {
	vCluster := virtualenv.NewControlPlane(appConfig.BinaryAssetsPath)
	if err := vCluster.Start(ctx); err != nil {
		slog.Error("failed to start virtual cluster", "error", err)
		os.Exit(1)
	}
	slog.Info("virtual cluster started successfully")
	return vCluster
}

func initializeGardenAccess(ctx context.Context, appConfig app.Config) garden.Access {
	slog.Info("initializing garden access ...", "garden", appConfig.Garden)
	gardenAccess, err := garden.NewAccess(appConfig.Garden)
	if err != nil {
		slog.Error("failed to create garden access", "error", err)
		os.Exit(1)
	}
	slog.Info("syncing reference nodes from shoot", "garden", appConfig.Garden, "referenceShoot", appConfig.ReferenceShoot)
	if err = gardenAccess.SyncReferenceNodes(ctx, appConfig.ReferenceShoot); err != nil {
		slog.Error("failed to sync reference nodes", "referenceShoot", appConfig.ReferenceShoot, "error", err)
		os.Exit(1)
	}
	return gardenAccess
}

func startScenarioExecutorEngine(gardenAccess garden.Access, vCluster virtualenv.ControlPlane, pricingAccess pricing.InstancePricingAccess, targetShootCoord *app.ShootCoordinate) simulation.Engine {
	scenarioExecutorEngine := simulation.NewExecutor(gardenAccess, vCluster, pricingAccess, targetShootCoord)
	slog.Info("Triggering start of scenario executor...")
	go func() {
		defer scenarioExecutorEngine.Shutdown()
		scenarioExecutorEngine.Run()
	}()
	return scenarioExecutorEngine
}

func setupSignalHandler() context.Context {
	quit := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		cancel()
		<-quit
		os.Exit(1)
	}()
	return ctx
}

func parseCmdArgs() (app.Config, error) {
	config := app.Config{
		TargetShoot: &app.ShootCoordinate{},
	}
	args := os.Args[1:]
	fs := flag.CommandLine
	fs.StringVar(&config.Garden, "garden", "", "name of the garden")
	fs.StringVar(&config.BinaryAssetsPath, "binary-assets-path", "", "path to the binary assets (kube-apiserver, etcd)")
	fs.StringVar(&config.ReferenceShoot.Project, "reference-shoot-project", "", "project of the reference shoot")
	fs.StringVar(&config.ReferenceShoot.Name, "reference-shoot-name", "", "name of the reference shoot")
	fs.StringVar(&config.TargetShoot.Project, "target-shoot-project", "", "project of the target shoot")
	fs.StringVar(&config.TargetShoot.Name, "target-shoot-name", "", "name of the target shoot")
	if err := fs.Parse(args); err != nil {
		return config, err
	}
	return config, nil
}
