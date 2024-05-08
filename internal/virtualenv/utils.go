package virtualenv

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/events"
	schedulerappconfig "k8s.io/kubernetes/cmd/kube-scheduler/app/config"
	"k8s.io/kubernetes/pkg/scheduler"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"
)

func createKubeconfigFileForRestConfig(restConfig *rest.Config) ([]byte, error) {
	clusters := make(map[string]*clientcmdapi.Cluster)
	clusters["default-controlPlane"] = &clientcmdapi.Cluster{
		Server:                   restConfig.Host,
		CertificateAuthorityData: restConfig.CAData,
	}
	contexts := make(map[string]*clientcmdapi.Context)
	contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  "default-controlPlane",
		AuthInfo: "default-user",
	}
	authInfos := make(map[string]*clientcmdapi.AuthInfo)
	authInfos["default-user"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: restConfig.CertData,
		ClientKeyData:         restConfig.KeyData,
	}
	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authInfos,
	}
	kubeConfigBytes, err := clientcmd.Write(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubeconfig for virtual enviroment: %w", err)
	}
	return kubeConfigBytes, nil
}

func loadSchedulerConfig() (*config.KubeSchedulerConfiguration, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	currDirAbsPath, err := filepath.Abs(currentDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for current directory: %w", err)
	}
	configPath := filepath.Join(currDirAbsPath, "virtualenv", "assets", "scheduler-config.yaml")
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read kube scheduler config file: %s: %w", configPath, err)
	}
	obj, gvk, err := scheme.Codecs.UniversalDecoder().Decode(configBytes, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode kube scheduler config file: %s: %w", configPath, err)
	}
	if cfgObj, ok := obj.(*config.KubeSchedulerConfiguration); ok {
		cfgObj.TypeMeta.APIVersion = gvk.GroupVersion().String()
		return cfgObj, nil
	}
	return nil, fmt.Errorf("couldn't decode as KubeSchedulerConfiguration, got %s: ", gvk)
}

func writeKubeConfig(kubeConfigBytes []byte) (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	kubeConfigDir := filepath.Join(currentDir, "tmp")
	if _, err = os.Stat(kubeConfigDir); os.IsNotExist(err) {
		err = os.Mkdir(kubeConfigDir, 0755)
		if err != nil {
			return "", err
		}
	}
	kubeConfigPath := filepath.Join(kubeConfigDir, "kubeconfig.yaml")
	if err = os.WriteFile(kubeConfigPath, kubeConfigBytes, 0644); err != nil {
		return "", err
	}
	slog.Info("KubeConfig to connect to Virtual Custer written to: " + kubeConfigPath)
	return kubeConfigPath, nil
}

func createSchedulerAppConfig(restCfg *rest.Config) (*schedulerappconfig.Config, error) {
	client, eventsClient, err := createSchedulerClients(restCfg)
	if err != nil {
		return nil, err
	}
	eventBroadcaster := events.NewEventBroadcasterAdapter(eventsClient)
	informerFactory := scheduler.NewInformerFactory(client, 0)
	dynClient := dynamic.NewForConfigOrDie(restCfg)
	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynClient, 0, corev1.NamespaceAll, nil)
	schedulerConfig, err := loadSchedulerConfig()
	if err != nil {
		return nil, err
	}
	return &schedulerappconfig.Config{
		ComponentConfig:    *schedulerConfig,
		Client:             client,
		InformerFactory:    informerFactory,
		DynInformerFactory: dynamicInformerFactory,
		EventBroadcaster:   eventBroadcaster,
		KubeConfig:         restCfg,
	}, nil
}

func createSchedulerClients(restCfg *rest.Config) (kubernetes.Interface, kubernetes.Interface, error) {
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create scheduler client: %w", err)
	}
	eventClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create scheduler event client: %w", err)
	}
	return client, eventClient, nil
}
