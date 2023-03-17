package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/rubble/pkg/ipam"
	"github.com/rubble/pkg/k8s"
	"github.com/rubble/pkg/neutron"
	"github.com/rubble/pkg/rpc"
	"github.com/rubble/pkg/storage"
	"github.com/rubble/pkg/utils"
)

type rubbleService struct {
	kubeConfig      string
	openstackConfig string
	cniBinPath      string

	neutronNet    string
	neutronSubNet string

	k8s           *k8s.K8s
	neutronClient *neutron.Client

	resourceDB  storage.Storage
	portManager ipam.ResourceManager

	rpc.UnimplementedRubbleBackendServer
}

//返回db中存储的pod信息，或者nil
func (s *rubbleService) getPodResource(ns, name string) (ipam.PodResources, error) {
	obj, err := s.resourceDB.Get(podInfoKey(ns, name))
	if err == nil {
		return obj.(ipam.PodResources), nil
	}
	if err == storage.ErrNotFound {
		return ipam.PodResources{}, nil
	}

	return ipam.PodResources{}, err
}

func (s *rubbleService) allocatePortIP(ctx *ipam.NetworkContext, old *ipam.PodResources) (*ipam.Port, error) {
	oldVethRes := old.GetResourceItemByType(utils.ResourceTypePort)
	oldVethId := ""
	if old.PodInfo != nil {
		if len(oldVethRes) == 0 {
			logger.Infof("eniip for pod %s is zero", podInfoKey(old.PodInfo.Namespace, old.PodInfo.Name))
		} else if len(oldVethRes) > 1 {
			logger.Infof("eniip for pod %s more than one", podInfoKey(old.PodInfo.Namespace, old.PodInfo.Name))
		} else {
			oldVethId = oldVethRes[0].ID
		}
	}

	res, err := s.portManager.Allocate(ctx, oldVethId)
	if err != nil {
		return nil, err
	}
	return res.(*ipam.Port), nil
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
	podInfo, err := s.k8s.GetPod(r.K8SPodNamespace, r.K8SPodName)
	if err != nil {
		return nil, fmt.Errorf("error get pod info for: %+v", err)
	}
	logger.Infof("********Pod is %s ******", podInfo)

	// 2. Find old resource info
	oldRes, err := s.getPodResource(podInfo.Namespace, podInfo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod resources from db for pod %s/%s with error: %w", podInfo.Namespace, podInfo.Name, err)
	}

	// 3. Allocate network resource for pod
	portContext := &ipam.NetworkContext{
		Context:    ctx,
		Resources:  []ipam.ResourceItem{},
		Pod:        podInfo,
	}

	port, err := s.allocatePortIP(portContext, &oldRes)
	if err != nil {
		return nil, fmt.Errorf("error get allocated port for: %+v, result: %w", podInfo, err)
	}
	newRes := ipam.PodResources{
		PodInfo: podInfo,
		Resources: []ipam.ResourceItem{
			{
				ID:   port.GetResourceId(),
				Type: port.GetType(),
			},
		},
	}
	err = s.resourceDB.Put(podInfoKey(podInfo.Namespace, podInfo.Name), newRes)
	if err != nil {
		return nil, fmt.Errorf("error put resource into store with error: %w", err)
	}

	conf, err := ipam.NetConfFromPort(port)
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

	// 4. grpc connection
	if ctx.Err() != nil {
		err = ctx.Err()
		return nil, fmt.Errorf("error:%w on grpc connection", err)
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

	k8s, err := k8s.NewK8s(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to init k8s client with error: %w", err)
	}

	netClient, err := neutron.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create neutron client with error: %w", err)
	}

	daemonConfig, err := getConfigFromPath(utils.DefaultDeamonConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed read config file with error: %w", err)
	}

	logger.Infof("Daemon config is %+v", *daemonConfig)

	resourceDB, err := storage.NewDiskStorage(
		utils.ResDBName, utils.ResDBPath, json.Marshal, func(bytes []byte) (interface{}, error) {
			resourceRel := &ipam.PodResources{}
			err = json.Unmarshal(bytes, resourceRel)
			if err != nil {
				return nil, fmt.Errorf("error unmarshal pod relate resource: %w", err)
			}
			return *resourceRel, nil
		})
	if err != nil {
		return nil, fmt.Errorf("error init resource manager storage: %w", err)
	}

	localResource := make(map[string][]string)
	resObjList, err := resourceDB.List()
	if err != nil {
		return nil, fmt.Errorf("error list resource relation db with error: %w", err)
	}
	logger.Infof("############# Resource from db is %+v", resObjList)
	for _, resObj := range resObjList {
		podRes := resObj.(ipam.PodResources)
		logger.Infof("############# Item from db is %+v", podRes)
		for _, res := range podRes.Resources {
			if localResource[res.Type] == nil {
				localResource[res.Type] = make([]string, 0)
			}
			localResource[res.Type] = append(localResource[res.Type], res.ID)
		}
	}
	logger.Infof("############# localResource is %+v", localResource)

	portManager, err := ipam.NewPortResourceManager(daemonConfig, netClient, localResource[utils.ResourceTypePort])
	if err != nil {
		return nil, fmt.Errorf("error init port resource manager: %w", err)
	}

	service := &rubbleService{
		kubeConfig:      kubeConfig,
		openstackConfig: openstackConfig,
		cniBinPath:      cniBinPath,
		neutronNet:      net,
		neutronSubNet:   subnet,
		k8s:             k8s,
		neutronClient:   netClient,

		resourceDB:  resourceDB,
		portManager: portManager,
	}

	return service, nil
}


func getConfigFromPath(path string) (*utils.DaemonConfigure, error) {
	config := &utils.DaemonConfigure{}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed open config file: %w", err)
	}

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed read file %s: %v", path, err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed parse config: %v", err)
	}

	return config, nil
}

func podInfoKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}
