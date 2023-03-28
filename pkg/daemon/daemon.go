package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rubble/pkg/utils"
	"io/ioutil"
	"os"

	"github.com/rubble/pkg/ipam"
	"github.com/rubble/pkg/k8s"
	"github.com/rubble/pkg/neutron"
	"github.com/rubble/pkg/rpc"
	"github.com/rubble/pkg/storage"
)

type daemonServer struct {
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
func (s *daemonServer) getPodResource(key string) (ipam.PodResources, error) {
	obj, err := s.resourceDB.Get(key)
	if err == nil {
		return obj.(ipam.PodResources), nil
	}
	if err == storage.ErrNotFound {
		return ipam.PodResources{}, nil
	}

	return ipam.PodResources{}, err
}

func (s *daemonServer) allocatePortIP(ctx *ipam.ResourceContext, old *ipam.PodResources) (*ipam.PortResource, error) {
	oldRes := old.GetResourceItemByType(utils.ResourceTypeMultipleIP)
	logger.Infof("@@@@@@@@@@@@@@@@ what is old resource for %v", oldRes)
	oldResId := ""
	if old.PodInfo != nil {
		if len(oldRes) == 0 {
			logger.Infof("eniip for pod %s is zero", old.PodInfo.PodInfoKey())
		} else if len(oldRes) > 1 {
			logger.Infof("eniip for pod %s more than one", old.PodInfo.PodInfoKey())
		} else {
			oldResId = oldRes[0].ID
		}
	}

	res, err := s.portManager.Allocate(ctx, oldResId)
	if err != nil {
		return nil, err
	}
	return res.(*ipam.PortResource), nil
}

func (s *daemonServer) AllocateIP(ctx context.Context, r *rpc.AllocateIPRequest) (*rpc.AllocateIPReply, error) {
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
	podInfo, pod, err := s.k8s.GetPod(r.K8SPodNamespace, r.K8SPodName)
	if err != nil {
		return nil, fmt.Errorf("error get pod info for: %+v", err)
	}
	logger.Infof("********Pod is %s ******", podInfo)

	// 2. Find old resource info
	oldRes, err := s.getPodResource(podInfo.PodInfoKey())
	if err != nil {
		return nil, fmt.Errorf("failed to get pod resources from db for pod %s with error: %w", podInfo.PodInfoKey(), err)
	}

	// 3. Allocate network resource for pod
	resContext := &ipam.ResourceContext{
		Context: ctx,
		PodInfo: podInfo,
		Pod:     pod,
	}

	port, err := s.allocatePortIP(resContext, &oldRes)
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
	logger.Infof("$$$$$$$$$$ PUT DB  %+v, %+v", newRes, newRes.PodInfo)
	err = s.resourceDB.Put(podInfo.PodInfoKey(), newRes)
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

func (s *daemonServer) ReleaseIP(ctx context.Context, r *rpc.ReleaseIPRequest) (*rpc.ReleaseIPReply, error) {
	return nil, nil
}

func (s *daemonServer) GetIPInfo(ctx context.Context, r *rpc.GetInfoRequest) (*rpc.GetInfoReply, error) {
	return nil, nil
}

func newDaemonServer(kubeConfig, openstackConfig, net, subnet string) (rpc.RubbleBackendServer, error) {
	cniBinPath := os.Getenv("CNI_PATH")
	if cniBinPath == "" {
		cniBinPath = utils.DefaultCNIPath
	}

	neutronService, err := neutron.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create neutron client with error: %w", err)
	}

	daemonConfig, err := getConfigFromPath(utils.DefaultDeamonConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed read config file with error: %w", err)
	}
	nodeInfo, err := getNodeInfo(neutronService)
	if err != nil {
		return nil, fmt.Errorf("failed get node info with error: %w", err)
	}
	if len(nodeInfo.Name) == 0 {
		nodeInfo.Name = daemonConfig.NodeName
	}
	daemonConfig.Node = nodeInfo
	logger.Infof("Daemon config is %+v", *daemonConfig)

	k8sService, err := k8s.NewK8s(kubeConfig, nodeInfo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to init k8s client with error: %w", err)
	}

	resourceDB, err := storage.NewDiskStorage(utils.ResDBName, utils.DaemonDBPath, json.Marshal, jsonDeserializer)
	if err != nil {
		return nil, fmt.Errorf("error init resource manager storage: %w", err)
	}

	filter := &k8s.Filter{
		Annotations: map[string]string{
			"rubble.kubernetes.io/network": "true",
		},
		Labels: map[string]string{
			"vpc-cni": "true",
		},
	}
	pods, err := k8sService.ListLocalPods(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list local pods with error: %w", err)
	}
	logger.Infof("Local pods is %+v", pods)
	podsUsage := getPodsWithoutPort(pods, resourceDB)

	portsMapping, err := getPortsMapping(podsUsage, resourceDB)
	if err != nil {
		return nil, fmt.Errorf("error get ports usage in db storage: %w", err)
	}

	portManager, err := ipam.NewPortResourceManager(daemonConfig, neutronService, portsMapping)
	if err != nil {
		return nil, fmt.Errorf("error init port resource manager: %w", err)
	}

	//(TODO) start gc
	// service.startGarbageCollectionLoop()
	// gc 处理 daemon boltdb 中记录的 pod 和 port对应关系 不匹配问题

	service := &daemonServer{
		kubeConfig:      kubeConfig,
		openstackConfig: openstackConfig,
		cniBinPath:      cniBinPath,
		neutronNet:      net,
		neutronSubNet:   subnet,
		k8s:             k8sService,
		neutronClient:   neutronService,

		resourceDB:  resourceDB,
		portManager: portManager,
	}

	return service, nil
}

func getNodeInfo(client *neutron.Client) (*utils.NodeInfo, error) {
	if !utils.IfRuningOnVM() {
		logger.Infof("########## Not running on VM, return fake nodeinfo")
		fakeNode := &utils.NodeInfo{
			UUID: "fake-uuid-from-rubble",
		}
		return fakeNode, nil
	}

	data, err := client.GetVMMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to get node withis: %w", err)
	}

	node := &utils.NodeInfo{}
	err = json.Unmarshal(data, node)
	if err != nil {
		return nil, err
	}

	logger.Infof("######## node is %+v", node)
	return node, nil
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

func jsonDeserializer(bytes []byte) (interface{}, error) {
	resourceRel := &ipam.PodResources{}
	err := json.Unmarshal(bytes, resourceRel)
	if err != nil {
		return nil, fmt.Errorf("error unmarshal pod relate resource: %w", err)
	}
	return *resourceRel, nil
}

func getPortsMapping(podsUsage map[string]*k8s.PodInfo, db storage.Storage) (map[string][]string, error) {
	resObjList, err := db.List()
	if err != nil {
		return nil, fmt.Errorf("error list resource relation db with error: %w", err)
	}
	logger.Infof("############# Resource from db is %+v", resObjList)

	for k, v := range podsUsage {
		logger.Infof("$$$$$$$$$$$$$ key is %s, value is %+v", k, v)
	}

	portPodMapping := make(map[string][]string)
	for _, res := range resObjList {
		mapping := res.(ipam.PodResources)
		logger.Infof("############# Item from db is %+v， %+v, %+v", res, mapping, *mapping.PodInfo)

		_, ok := podsUsage[mapping.PodInfo.PodInfoKey()]
		if !ok {
			logger.Infof("!!!!!!!!!!!! GC REQUIRED!!!!! pod %s is not running on nodes, but using port %s in db", mapping.PodInfo.PodInfoKey(), mapping.Resources[0].ID)
		}

		for _, port := range mapping.Resources {
			if portPodMapping[port.ID] == nil {
				portPodMapping[port.ID] = make([]string, 0)
			}
			portPodMapping[port.ID] = append(portPodMapping[port.ID], mapping.PodInfo.PodInfoKey())
		}
	}
	return portPodMapping, nil
}

func getPodsWithoutPort(pods []*k8s.PodInfo, db storage.Storage) map[string]*k8s.PodInfo {
	podMaps := make(map[string]*k8s.PodInfo)

	for _, p := range pods {
		obj, err := db.Get(p.PodInfoKey())
		if err == nil {
			podMaps[p.PodInfoKey()] = p
			pod := *obj.(ipam.PodResources).PodInfo
			logger.Infof("########## get pod %s from db value is %+v, pod inf okey is: %s", p.PodInfoKey(), pod, pod.PodInfoKey())
		}
		if err == storage.ErrNotFound {
			logger.Infof("!!!!!!!!! pod %s is not using port recored in db, pod ip is %s", p.PodInfoKey(), p.PodIP)
		}

	}
	return podMaps
}
