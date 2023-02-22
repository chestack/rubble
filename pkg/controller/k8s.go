package controller

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func initKubeClient(kubeConf string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	if kubeConf == "" {
		logger.Infof("no --kubeconfig, use in-cluster kubernetes config")
		config, err = rest.InClusterConfig()
		if err != nil {
			logger.Errorf("use in cluster config failed %v", err)
			return nil, err
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeConf)
		if err != nil {
			logger.Errorf("use --kubeconfig %s failed %v", kubeConf, err)
			return nil, err
		}
	}
	config.QPS = 1000
	config.Burst = 2000

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Failed to create Kubernetes client: %v", err)
		return nil, err
	}
	return client, nil
}