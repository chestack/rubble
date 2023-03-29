package k8s

import (
	"context"
	"fmt"
	types "github.com/rubble/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/rubble/pkg/log"
)

const defaultStickTimeForSts = 5 * time.Minute

var workloadSet = sets.NewString("statefulset")

// PodInfo (fixme) https://stackoverflow.com/questions/21825322/why-golang-cannot-generate-json-from-struct-with-front-lowercase-character
type PodInfo struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	PodIP       string `json:"pod_ip"`
	IpStickTime time.Duration
}

func (p *PodInfo) PodInfoKey() string {
	return fmt.Sprintf("%s/%s", p.Namespace, p.Name)
}

var logger = log.DefaultLogger.WithField("component:", "rubble cni-server")

type K8s struct {
	client   *kubernetes.Clientset
	nodeName string
	nodeCidr *net.IPNet
	svcCidr  *net.IPNet
}

type Filter struct {
	Annotations map[string]string
	Labels      map[string]string
}

func NewK8s(conf string, nodeName string) (*K8s, error) {

	client, err := initKubeClient(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create neutron client with error: %w", err)
	}

	return &K8s{
		client:   client,
		nodeName: nodeName,
	}, nil

	return nil, nil
}

func (k *K8s) GetPod(namespace, name string) (*PodInfo, *corev1.Pod, error) {
	pod, err := k.client.CoreV1().Pods(namespace).Get(context.Background(), name, v1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	podInfo := convertPod(pod)
	return podInfo, pod, nil
}

func (k *K8s) ListLocalPods(filter *Filter) ([]*PodInfo, error) {
	var selectors []fields.Selector
	selectors = append(selectors, fields.OneTermEqualSelector("spec.nodeName", k.nodeName))

	//(TODO) failed to list pods with annotations
	//for key, value := range filter.Annotations {
	//	selectors = append(selectors, fields.OneTermEqualSelector(key, value))
	//}

	options := v1.ListOptions{
		LabelSelector: v1.FormatLabelSelector(v1.SetAsLabelSelector(filter.Labels)),
		FieldSelector: fields.AndSelectors(selectors...).String(),
	}
	list, err := k.client.CoreV1().Pods(corev1.NamespaceAll).List(context.Background(), options)
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
		Name:      pod.Name,
		Namespace: pod.Namespace,
		PodIP:     pod.Status.PodIP,
	}

	// determine whether pod's IP will stick 5 minutes for a reuse
	switch {
	case parseBool(pod.Annotations[types.PodStaticIP]):
		pi.IpStickTime = defaultStickTimeForSts
	case len(pod.OwnerReferences) > 0:
		for i := range pod.OwnerReferences {
			if workloadSet.Has(strings.ToLower(pod.OwnerReferences[i].Kind)) {
				pi.IpStickTime = defaultStickTimeForSts
				break
			}
		}
	}

	return pi
}

func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
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
