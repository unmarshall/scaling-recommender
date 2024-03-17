package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx := setupSignalHandler()

	<-ctx.Done()
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
