package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"unmarshall/scaling-recommender/api"
	"unmarshall/scaling-recommender/internal/scaler"
	"unmarshall/scaling-recommender/internal/simulation"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"unmarshall/scaling-recommender/internal/app"
)

func main() {
	defer app.OnExit()
	utilruntime.Must(gardencorev1beta1.AddToScheme(scheme.Scheme))
	utilruntime.Must(seedmanagementv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(machinev1alpha1.AddToScheme(scheme.Scheme))
	ctx := setupSignalHandler()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	appConfig, err := parseCmdArgs()
	if err != nil {
		app.ExitAppWithError(1, fmt.Errorf("failed to parse command line arguments: %w", err))
	}

	if err = validateConfig(appConfig); err != nil {
		app.ExitAppWithError(1, fmt.Errorf("invalid configuration: %w", err))
	}

	logger.Info("starting scaling recommender environment", "appConfig", appConfig)
	startExecutorEngine(ctx, appConfig, logger)
	<-ctx.Done()
}

func startExecutorEngine(ctx context.Context, appConfig api.AppConfig, logger *slog.Logger) {
	engine := simulation.NewExecutorEngine(appConfig, logger)
	go func() {
		defer engine.Shutdown()
		if err := engine.Start(ctx); err != nil {
			app.ExitAppWithError(1, fmt.Errorf("error starting executor engine: %w", err))
		}
	}()
	return
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

func parseCmdArgs() (api.AppConfig, error) {
	config := api.AppConfig{}
	args := os.Args[1:]
	fs := flag.CommandLine

	fs.StringVar(&config.BinaryAssetsPath, "binary-assets-path", "", "path to the binary assets (kube-apiserver, etcd)")
	fs.StringVar(&config.Provider, "provider", "", "provider of the target shoot")
	fs.StringVar(&config.TargetKVCLKubeConfigPath, "target-kvcl-kubeconfig", "", "path to the kubeconfig of the target cluster")
	fs.StringVar(&config.ScoringStrategy, "scoring-strategy", string(scaler.CostOnlyStrategy), "scoring strategy")

	if err := fs.Parse(args); err != nil {
		return config, err
	}
	return config, nil
}

func validateConfig(config api.AppConfig) error {
	if config.BinaryAssetsPath == "" {
		return fmt.Errorf("binary assets path is required")
	}
	if config.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if config.TargetKVCLKubeConfigPath == "" {
		return fmt.Errorf("kubeconfig path is required")
	}
	if !scaler.IsScoringStrategySupported(config.ScoringStrategy) {
		return fmt.Errorf("scoring strategy %s is not supported", config.ScoringStrategy)
	}
	return nil
}
