package k8s

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
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
	name           string
	namespace      string
	podIP          string
	ipStickTime    time.Duration
}

func (p *PodInfo) PodInfoKey() string {
	return fmt.Sprintf("%s/%s", p.namespace, p.name)
}

func (p *PodInfo) GetPodIP() string {
	return p.podIP
}

func (p *PodInfo) GetPodIPStickTime() time.Duration {
	return p.ipStickTime
}

var logger = log.DefaultLogger.WithField("component:", "rubble cni-server")

type K8s struct {
	client   *kubernetes.Clientset
	nodeName string
	nodeCidr *net.IPNet
	svcCidr  *net.IPNet
}

func NewK8s(conf string,  nodeName string) (*K8s, error) {

	client, err := initKubeClient(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create neutron client with error: %w", err)
	}

	return &K8s{
		client: client,
		nodeName: nodeName,
	}, nil

	return nil, nil
}

func (k *K8s) GetPod(namespace, name string) (*PodInfo, error) {
	pod, err := k.client.CoreV1().Pods(namespace).Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	podInfo := convertPod(pod)
	return podInfo, nil
}

func (k *K8s) ListLocalPods() ([]*PodInfo, error) {
	options := v1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", k.nodeName).String(),
	}
	list, err := k.client.CoreV1().Pods(corev1.NamespaceAll).List(context.Background(),options)
	if err != nil {
		return nil, fmt.Errorf("failed listting pods on node:%s from apiserver with error: %w", k.nodeName, err)
	}
	var ret []*PodInfo
	for _, pod := range list.Items {
		info := convertPod(&pod)
		ret = append(ret, info)
	}

	return ret, nil
}

func convertPod(pod *corev1.Pod) *PodInfo {

	pi := &PodInfo{
		name:      pod.Name,
		namespace: pod.Namespace,
	}

	pi.podIP = pod.Status.PodIP

	if len(pod.OwnerReferences) != 0 {
		switch strings.ToLower(pod.OwnerReferences[0].Kind) {
		case "statefulset":
			pi.ipStickTime = defaultStickTimeForSts
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