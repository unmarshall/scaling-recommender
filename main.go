package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"unmarshall/scaling-recommender/virtualenv"
)

func main() {
	ctx := setupSignalHandler()
	binaryAssetsPath := "/Users/i062009/Library/Application Support/io.kubebuilder.envtest/k8s/1.29.1-darwin-arm64"
	vCluster := virtualenv.NewCluster(binaryAssetsPath)
	if err := vCluster.Start(ctx); err != nil {
		slog.Error("failed to start virtual cluster", "error", err)
		os.Exit(1)
	}
	slog.Info("virtual cluster started successfully")
	<-ctx.Done()
	if err := vCluster.Stop(); err != nil {
		slog.Error("failed to stop virtual cluster", "error", err)
	}
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
