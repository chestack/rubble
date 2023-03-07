package controller

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"

	"github.com/rubble/pkg/neutron"
	"github.com/rubble/pkg/rpc"
	"github.com/rubble/pkg/utils"

	"k8s.io/client-go/kubernetes"
)

type rubbleService struct {
	kubeConfig      string
	openstackConfig string
	cniBinPath      string

	neutronNet    string
	neutronSubNet string

	k8sClient     kubernetes.Interface
	neutronClient *neutron.Client

	rpc.UnimplementedRubbleBackendServer
}

func (s *rubbleService) AllocateIP(ctx context.Context, r *rpc.AllocateIPRequest) (*rpc.AllocateIPReply, error) {
	logger.Infof("Do Allocate IP with request %+v", r)
	logger.Infof("********do some allocating work******")

	podName := fmt.Sprintf("%s/%s", r.K8SPodNamespace, r.K8SPodName)
	logger.WithFields(map[string]interface{}{
		"pod":         podName,
		"containerID": r.K8SPodInfraContainerId,
		"netNS":       r.Netns,
		"ifName":      r.IfName,
	}).Info("alloc ip req")

	// 1. get pod Info
	podInfo, err := s.k8sClient.CoreV1().Pods(r.K8SPodNamespace).Get(context.Background(), r.K8SPodName, v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get pod info for: %+v", err)
	}
	logger.Infof("********Pod is %s ******", podInfo)

	netId, err := s.neutronClient.GetNetworkID(s.neutronNet)
	if err != nil {
		return nil, fmt.Errorf("failed to get network id with: %s, error is: %w", s.neutronNet, err)
	}
	logger.Infof("********Net ID is: %s ******", netId)

	subnetId, err := s.neutronClient.GetSubnetworkID(s.neutronSubNet)
	if err != nil {
		return nil, fmt.Errorf("failed to get subnet with: %s, error is: %w", s.neutronSubNet, err)
	}
	logger.Infof("********Sub Net ID is: %s ******", subnetId)

	// 2. create port
	opts := neutron.CreateOpts{
		Name:      fmt.Sprintf("rubble-%s/%s", podInfo.Namespace, podInfo.Name),
		NetworkID: netId,
		SubnetID:  subnetId,
	}
	port, err := s.neutronClient.CreatePort(&opts)
	if err != nil {
		logger.Errorf("failed to create port with error: %s", err)
		return nil, err
	}

	logger.Infof("********port is %+v ******", port)

	conf, err := netConfFromPort(port, podInfo)
	if err != nil {
		logger.Errorf("failed to generate net config with error: %s", err)
		return nil, err
	}

	allocIPReply := &rpc.AllocateIPReply{
		Success:  true,
		IPType:   rpc.IPType_TypeENIMultiIP,
		IPv4:     true,
		NetConfs: conf,
	}

	return allocIPReply, err
}

func (s *rubbleService) ReleaseIP(ctx context.Context, r *rpc.ReleaseIPRequest) (*rpc.ReleaseIPReply, error) {
	return nil, nil
}

func (s *rubbleService) GetIPInfo(ctx context.Context, r *rpc.GetInfoRequest) (*rpc.GetInfoReply, error) {
	return nil, nil
}

func newRubbleService(kubeConfig, openstackConfig, net, subnet string) (rpc.RubbleBackendServer, error) {
	cniBinPath := os.Getenv("CNI_PATH")
	if cniBinPath == "" {
		cniBinPath = utils.DefaultCNIPath
	}

	k8s, err := initKubeClient(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to init k8s client with error: %w", err)
	}

	netClient, err := neutron.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create neutron client with error: %w", err)
	}

	service := &rubbleService{
		kubeConfig:      kubeConfig,
		openstackConfig: openstackConfig,
		cniBinPath:      cniBinPath,
		neutronNet:      net,
		neutronSubNet:   subnet,
		k8sClient:       k8s,
		neutronClient:   netClient,
	}

	return service, nil
}

func netConfFromPort(port neutron.Port, pod *corev1.Pod) ([]*rpc.NetConf, error) {
	var netConf []*rpc.NetConf

	// call api to get eni info
	podIP := &rpc.IPSet{}
	cidr := &rpc.IPSet{}
	gw := &rpc.IPSet{}

	podIP.IPv4 = port.IP
	cidr.IPv4 = port.CIDR
	gw.IPv4 = port.Gateway

	if cidr.IPv4 == "" || gw.IPv4 == "" {
		return nil, fmt.Errorf("empty cidr or gateway")
	}

	eniInfo := &rpc.ENIInfo{
		MAC: port.MAC,
		GatewayIP: &rpc.IPSet{
			IPv4: port.Gateway,
		},
	}

	netConf = append(netConf, &rpc.NetConf{
		BasicInfo: &rpc.BasicInfo{
			PodIP:     podIP,
			PodCIDR:   cidr,
			GatewayIP: gw,
		},
		ENIInfo: eniInfo,
	})

	return netConf, nil
}
