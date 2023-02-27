package controller

import (
	"context"
	"fmt"
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

	k8sClient kubernetes.Interface
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

	// 0. Get pod Info
	podInfo, err := s.k8sClient.CoreV1().Pods(r.K8SPodNamespace).Get(context.Background(), r.K8SPodName, v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get pod info for: %+v", err)
	}
	logger.Infof("********Pod is %s ******", podInfo)

	allocIPReply := &rpc.AllocateIPReply{}
	return allocIPReply, err
}

func (s *rubbleService) ReleaseIP(ctx context.Context, r *rpc.ReleaseIPRequest) (*rpc.ReleaseIPReply, error) {
	return nil, nil
}

func (s *rubbleService) GetIPInfo(ctx context.Context, r *rpc.GetInfoRequest) (*rpc.GetInfoReply, error) {
	return nil, nil
}

func newRubbleService(kubeConfig, openstackConfig string) (rpc.RubbleBackendServer, error) {
	cniBinPath := os.Getenv("CNI_PATH")
	if cniBinPath == "" {
		cniBinPath = utils.DefaultCNIPath
	}

	k8s, err := initKubeClient(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to init k8s client with error: %w", err)
	}

	service := &rubbleService{
		kubeConfig:      kubeConfig,
		openstackConfig: openstackConfig,
		cniBinPath:      cniBinPath,
		k8sClient:       k8s,
		neutronClient:   neutron.NewClient(),
	}

	return service, nil
}
