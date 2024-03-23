package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	resourcemanagerv1alpha1 "github.com/gardener/gardener/pkg/resourcemanager/apis/config/v1alpha1"
	"github.com/gardener/gardener/pkg/resourcemanager/controller/garbagecollector/references"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	garden2 "unmarshall/scaling-recommender/internal/garden"
)

func main() {
	utilruntime.Must(gardencorev1beta1.AddToScheme(scheme.Scheme))
	utilruntime.Must(seedmanagementv1alpha1.AddToScheme(scheme.Scheme))
	ga, err := garden2.NewAccess("sap-landscape-live")
	dieOnError(err, "failed to create garden access")
	ctx := context.Background()
	for _, t := range getTargets() {
		seedAccess, err := ga.GetShootAccess(ctx, t.seedCoord)
		dieOnError(err, fmt.Sprintf("failed to get seed access: %v", t.seedCoord))
		cl := seedAccess.GetClient()
		for _, shootControlPlaneNs := range t.shootControlNamespaces {
			newCm, err := getNewGRMConfigMap(ctx, cl, shootControlPlaneNs)
			dieOnError(err, fmt.Sprintf("failed to get new GRM config map in namespace %s", shootControlPlaneNs))
			grmDeployment, err := getGrmDeployment(ctx, cl, shootControlPlaneNs)
			dieOnError(err, fmt.Sprintf("failed to get GRM deployment in namespace %s", shootControlPlaneNs))
			if newCm == nil {
				// get current configmap
				existingCmName, _ := getGRMCmVolumeNameAndIndex(grmDeployment)
				existingCm, err := getConfigMap(ctx, cl, shootControlPlaneNs, existingCmName)
				dieOnError(err, fmt.Sprintf("failed to get existing GRM config map in namespace %s", shootControlPlaneNs))
				newCm = createNewCmFromOld(existingCm)
				if newCm != nil {
					err := cl.Create(ctx, newCm)
					dieOnError(err, fmt.Sprintf("failed to create new GRM config map name: %s in namespace %s", newCm.Name, newCm.Namespace))
				}
			}
			err = updateGrmDeploymentWithConfigMapVol(ctx, cl, grmDeployment, newCm.Name)
			dieOnError(err, fmt.Sprintf("failed to update GRM deployment in namespace %s", shootControlPlaneNs))
		}
	}

}

func createNewCmFromOld(oldCm *corev1.ConfigMap) *corev1.ConfigMap {
	cmData := oldCm.Data["config.yaml"]
	codec := createCodec()
	obj, err := runtime.Decode(codec, []byte(cmData))
	dieOnError(err, fmt.Sprintf("failed to decode config map data for name: %s, namespace: %s", oldCm.Name, oldCm.Namespace))
	rmConfig := *(obj.(*resourcemanagerv1alpha1.ResourceManagerConfiguration))
	if rmConfig.TargetClientConnection == nil || slices.Contains(rmConfig.TargetClientConnection.Namespaces, corev1.NamespaceNodeLease) {
		return nil
	}
	rmConfig.TargetClientConnection.Namespaces = append(rmConfig.TargetClientConnection.Namespaces, corev1.NamespaceNodeLease)
	data, err := runtime.Encode(codec, &rmConfig)
	dieOnError(err, fmt.Sprintf("failed to encode config map data for name: %s, namespace: %s", oldCm.Name, oldCm.Namespace))
	newGRMConfigMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "gardener-resource-manager-dwd", Namespace: oldCm.Namespace}}
	newGRMConfigMap.Data = map[string]string{"config.yaml": string(data)}
	utilruntime.Must(kubernetesutils.MakeUnique(newGRMConfigMap))
	return newGRMConfigMap
}

func createCodec() runtime.Codec {
	configScheme := runtime.NewScheme()
	utilruntime.Must(resourcemanagerv1alpha1.AddToScheme(configScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(configScheme))
	ser := json.NewSerializerWithOptions(json.DefaultMetaFactory, configScheme, configScheme, json.SerializerOptions{
		Yaml:   true,
		Pretty: false,
		Strict: false,
	})
	versions := schema.GroupVersions([]schema.GroupVersion{
		resourcemanagerv1alpha1.SchemeGroupVersion,
		apiextensionsv1.SchemeGroupVersion,
	})
	return serializer.NewCodecFactory(configScheme).CodecForVersions(ser, ser, versions, versions)
}

func getConfigMap(ctx context.Context, cl client.Client, ns string, name string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	if err := cl.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, cm); err != nil {
		return nil, err
	}
	return cm, nil
}

func getGRMCmVolumeNameAndIndex(grmDeployment *appsv1.Deployment) (string, int) {
	volumes := grmDeployment.Spec.Template.Spec.Volumes
	for i, v := range volumes {
		if v.Name == "config" {
			return v.ConfigMap.Name, i
		}
	}
	return "", -1
}

func updateGrmDeploymentWithConfigMapVol(ctx context.Context, cl client.Client, grmDeployment *appsv1.Deployment, newCmName string) error {
	cmName, index := getGRMCmVolumeNameAndIndex(grmDeployment)
	if cmName == newCmName {
		return nil
	}
	patch := client.MergeFrom(grmDeployment.DeepCopy())
	grmDeployment.Spec.Template.Spec.Volumes[index].ConfigMap.Name = newCmName
	utilruntime.Must(references.InjectAnnotations(grmDeployment))

	return cl.Patch(ctx, grmDeployment, patch)
}

func getGrmDeployment(ctx context.Context, cl client.Client, ns string) (*appsv1.Deployment, error) {
	dep := &appsv1.Deployment{}
	if err := cl.Get(ctx, client.ObjectKey{Name: "gardener-resource-manager", Namespace: ns}, dep); err != nil {
		return nil, err
	}
	return dep, nil
}

func getNewGRMConfigMap(ctx context.Context, cl client.Client, ns string) (*corev1.ConfigMap, error) {
	const grmDWDConfigMapNamePrefix = "gardener-resource-manager-dwd"
	cmList := &corev1.ConfigMapList{}
	if err := cl.List(ctx, cmList, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	for _, cm := range cmList.Items {
		if strings.HasPrefix(cm.Name, grmDWDConfigMapNamePrefix) {
			return &cm, nil
		}
	}
	return nil, nil
}

type target struct {
	seedCoord              garden2.ShootCoordinates
	shootControlNamespaces []string
}

func getTargets() []target {
	return []target{
		{
			seedCoord:              garden2.ShootCoordinates{Project: "garden", Name: "gcp-us1"},
			shootControlNamespaces: []string{"shoot--als-canary--cf-us31"},
		},
		{
			seedCoord:              garden2.ShootCoordinates{Project: "garden", Name: "aws-ha-ap3"},
			shootControlNamespaces: []string{"shoot--als-global--aws-ap12"},
		},
		{
			seedCoord:              garden2.ShootCoordinates{Project: "garden", Name: "aws-ha-ap1"},
			shootControlNamespaces: []string{"shoot--als-global--aws-ap11"},
		},
		{
			seedCoord:              garden2.ShootCoordinates{Project: "garden", Name: "aws-ap3"},
			shootControlNamespaces: []string{"shoot--als-global--aws-jp10", "shoot--edgelm--prod-jp10"},
		},
		{
			seedCoord:              garden2.ShootCoordinates{Project: "garden", Name: "gcp-us1"},
			shootControlNamespaces: []string{"shoot--als-global--gcp-us30"},
		},
	}

}

func dieOnError(err error, msg string) {
	if err != nil {
		panic(fmt.Sprintf("%s, err: %v", msg, err))
	}
}
