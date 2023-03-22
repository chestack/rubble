package ipam

import (
	"fmt"
	"github.com/rubble/pkg/rpc"

	"github.com/rubble/pkg/log"
	"github.com/rubble/pkg/neutron"
	"github.com/rubble/pkg/pool"
	types "github.com/rubble/pkg/utils"
	"sync"
)

const (
	VMTag = "vm_uuid:"
	DeviceOwner = "network:secondary"
)

var logger = log.DefaultLogger.WithField("component:", "port resource manager")

type Port struct {
	port *neutron.Port
}

type PortFactory struct {
	neutronClient *neutron.Client
	netID         string
	subnetID      string
	nodeName      string
	vmUUID        string
	projectID     string
	ports         []*Port
	sync.RWMutex
}

func (p *Port) GetResourceId() string {
	return p.port.ID
}

func (p *Port) GetType() string {
	return types.ResourceTypePort
}

func (f *PortFactory) Create() (types.NetworkResource, error) {
	opts := neutron.CreateOpts{
		Name:      fmt.Sprintf("rubble-port-%s", types.RandomString(10)),
		NetworkID: f.netID,
		SubnetID:  f.subnetID,
		DeviceOwner: DeviceOwner,
	}
	port, err := f.neutronClient.CreatePort(&opts)
	if err != nil {
		logger.Errorf("failed to create port with error: %s", err)
		return nil, err
	}

	err = f.neutronClient.AddTag("ports", port.ID, fmt.Sprintf("%s/%s", VMTag, f.vmUUID))
	if err != nil {
		fmt.Errorf("failed to add tag to port:%s with error %w", port.ID, err)
		return nil, err
	}

	p := &Port{
		port: &port,
	}

	f.Lock()
	f.ports = append(f.ports, p)
	f.Unlock()

	return p, nil
}

func (f *PortFactory) Dispose(res types.NetworkResource) (err error) {
	defer func() {
		logger.Debugf("dispose result: %v, error: %v", res.GetResourceId(), err != nil)
	}()

	f.Lock()
	err = f.neutronClient.DeletePort(res.GetResourceId())
	if err != nil {
		return fmt.Errorf("failed to delete port with error: %w", err)
	}
	f.Unlock()
	return nil
}

type PortResourceManager struct {
	pool pool.ObjectPool
}

func NetConfFromPort(p *Port) ([]*rpc.NetConf, error) {
	var netConf []*rpc.NetConf

	port := p.port
	logger.Infof("############# neutron port is %+v and value is %+v", port, *port)
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

func NewPortResourceManager(config *types.DaemonConfigure, client *neutron.Client, portsMapping map[string][]string) (ResourceManager, error) {

	netId, err := client.GetNetworkID(config.NetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get network id with: %s, error is: %w", config.NetID, err)
	}
	logger.Infof("********Net ID is: %s ******", netId)

	subnetId, err := client.GetSubnetworkID(config.SubnetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subnet with: %s, error is: %w", config.SubnetID, err)
	}
	logger.Infof("********Sub Net ID is: %s ******", subnetId)

	factory := &PortFactory{
		neutronClient: client,
		netID:         netId,
		subnetID:      subnetId,
		nodeName:      config.Node.Name,
		vmUUID:        config.Node.UUID,
		projectID:     config.Node.ProjectID,
		ports:         []*Port{},
	}

	poolCfg := pool.PoolConfig{
		MaxIdle:     config.MaxIdleSize,
		MinIdle:     config.MinIdleSize,
		MaxPoolSize: config.MaxPoolSize,
		MinPoolSize: config.MinPoolSize,
		Capacity:    config.MaxPoolSize,

		Factory: factory,
		Initializer: func(holder pool.ResourceHolder) error {
			//(TODO) 把initializer 放到外面

			// get all ports assigned to this node
			ports, err := client.ListPortWithFilter(netId, DeviceOwner, fmt.Sprintf("%s/%s", VMTag, config.Node.UUID))
			if err != nil {
				return fmt.Errorf("failed to list ports allocated by this node %s with error: %w", config.Node.Name, err)
			}

			// loop ports to initialize pool inUse and idle
			for _, port := range ports {
				pod, ok := portsMapping[port.ID]
				p := &Port{
					port: client.ConvertPort(port),
				}

				// update ports list in factory
				factory.ports = append(factory.ports, p)

				if ok {
					logger.Infof("** port %s in using by pod %s, add it into insue", port.ID, pod)
					holder.AddInuse(p)
				} else {
					logger.Infof("** port %s is not using by any pod add it into idle", port.ID)
					holder.AddIdle(p)
				}

			}
			return nil
		},
	}
	pool, err := pool.NewSimpleObjectPool(poolCfg)
	if err != nil {
		return nil, err
	}

	mgr := &PortResourceManager{
		pool: pool,
	}

	return mgr, nil
}

func (m *PortResourceManager) Allocate(ctx *NetworkContext, prefer string) (types.NetworkResource, error) {
	return m.pool.Acquire(ctx.Context, prefer)
}

func (m *PortResourceManager) Release(ctx *NetworkContext, resId string) error {
	if ctx != nil && ctx.Pod != nil {
		logger.Infof("@@@@@@@@@@@ POd is %s, stick time is %s", ctx.Pod.PodInfoKey(), ctx.Pod.GetPodIPStickTime())
		return m.pool.ReleaseWithReverse(resId, ctx.Pod.GetPodIPStickTime())
	}
	return m.pool.Release(resId)
}

func (m *PortResourceManager) GarbageCollection(inUseSet map[string]interface{}, expireResSet map[string]interface{}) error {
	for expireRes := range expireResSet {
		if err := m.pool.Stat(expireRes); err == nil {
			err = m.Release(nil, expireRes)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
