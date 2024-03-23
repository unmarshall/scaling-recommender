package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"unmarshall/scaling-recommender/internal/simulation/executor"
	"unmarshall/scaling-recommender/internal/virtualenv"
)

func main() {
	utilruntime.Must(gardencorev1beta1.AddToScheme(scheme.Scheme))
	utilruntime.Must(seedmanagementv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(machinev1alpha1.AddToScheme(scheme.Scheme))
	ctx := setupSignalHandler()
	// take this as a CLI flag
	binaryAssetsPath := "/Users/i062009/Library/Application Support/io.kubebuilder.envtest/k8s/1.29.1-darwin-arm64"
	vCluster := virtualenv.NewControlPlane(binaryAssetsPath)
	if err := vCluster.Start(ctx); err != nil {
		slog.Error("failed to start virtual cluster", "error", err)
		os.Exit(1)
	}
	slog.Info("virtual cluster started successfully")
	scenarioExecutor := executor.NewExecutor(vCluster)
	go scenarioExecutor.Run()

	<-ctx.Done()
	slog.Info("shutting down virtual cluster...")
	if err := vCluster.Stop(); err != nil {
		slog.Error("failed to stop virtual cluster", "error", err)
	}
	slog.Info("shutting down scenario executor...")
	scenarioExecutor.Shutdown()
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
