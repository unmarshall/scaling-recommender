package main

import (
	"context"
	"log/slog"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"unmarshall/scaling-recommender/garden"
)

func main() {
	utilruntime.Must(gardencorev1beta1.AddToScheme(scheme.Scheme))
	utilruntime.Must(seedmanagementv1alpha1.AddToScheme(scheme.Scheme))
	ctx := context.Background()
	ga, err := garden.NewAccess("sap-landscape-dev")
	dieOnErr(err, "cannot create garden access")
	shootCoord := garden.ShootCoordinates{
		Project: "i062009",
		Shoot:   "reference",
	}
	shoot, err := ga.GetShoot(ctx, shootCoord)
	dieOnErr(err, "cannot get shoot")
	slog.Info("found shoot", "name", shoot.Name, "namespace", shoot.Namespace)
	sa, err := ga.GetShootAccess(ctx, shootCoord)
	dieOnErr(err, "cannot get shoot access")
	nodes, err := sa.GetNodes(ctx)
	if err != nil {
		return
	}
	for _, node := range nodes {
		slog.Info("node", "name", node.Name)
	}
}

func dieOnErr(err error, msg string) {
	if err != nil {
		slog.Error(msg, "error", err)
		panic(err)
	}
}
