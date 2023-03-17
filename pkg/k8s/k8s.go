package k8s

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net"
	"strings"
	"time"

	"github.com/rubble/pkg/log"
)

const defaultStickTimeForSts = 5 * time.Minute

type PodInfo struct {
	//K8sPod *v1.Pod
	Name           string
	Namespace      string
	PodIP          string
	IpStickTime    time.Duration
}

var logger = log.DefaultLogger.WithField("component:", "rubble cni-server")

type K8s struct {
	Client   *kubernetes.Clientset
	NodeName string
	NodeCidr *net.IPNet
	SvcCidr  *net.IPNet
}

func NewK8s(conf string) (*K8s, error) {

	client, err := initKubeClient(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create neutron client with error: %w", err)
	}

	return &K8s{
		Client: client,
	}, nil

	return nil, nil
}

func (k *K8s) GetPod(namespace, name string) (*PodInfo, error) {
	pod, err := k.Client.CoreV1().Pods(namespace).Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	podInfo := convertPod(pod)
	return podInfo, nil
}

func convertPod(pod *corev1.Pod) *PodInfo {

	pi := &PodInfo{
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}

	pi.PodIP = pod.Status.PodIP

	if len(pod.OwnerReferences) != 0 {
		switch strings.ToLower(pod.OwnerReferences[0].Kind) {
		case "statefulset":
			pi.IpStickTime = defaultStickTimeForSts
			break
		}
	}

	return pi
}

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

func podInfoKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}