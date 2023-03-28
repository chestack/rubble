package ipam

import (
	"fmt"
	"github.com/rubble/pkg/rpc"
	"net"

	"github.com/rubble/pkg/log"
	"github.com/rubble/pkg/neutron"
	"github.com/rubble/pkg/pool"
	types "github.com/rubble/pkg/utils"
	"sync"
)

const (
	VMTagPrefix = "vm_uuid"
	DeviceOwner = "network:secondary"

	IpAddressAnnotation = "rubble.kubernetes.io/ip_address"
	IpPoolAnnotation    = "rubble.kubernetes.io/ip_pool"
)

var logger = log.DefaultLogger.WithField("component:", "port resource manager")

type PortResource struct {
	port *neutron.Port
}

type PortFactory struct {
	client    *neutron.Client
	netID     string
	subnetID  string
	nodeName  string
	vmUUID    string
	projectID string
	ports     []*PortResource
	sync.RWMutex
}

func (p *PortResource) GetResourceId() string {
	return p.port.ID
}

func (p *PortResource) GetType() string {
	return types.ResourceTypeMultipleIP
}

func (p *PortResource) GetIPAddress() string {
	return p.port.IP
}

func (f *PortFactory) Create(ip string) (types.NetworkResource, error) {
	opts := neutron.CreateOpts{
		Name:        fmt.Sprintf("rubble-port-%s", types.RandomString(10)),
		NetworkID:   f.netID,
		SubnetID:    f.subnetID,
		DeviceOwner: DeviceOwner,
	}

	if len(ip) > 0 {
		opts.IPAddress = ip
	}

	port, err := f.client.CreatePort(&opts)
	if err != nil {
		logger.Errorf("failed to create port with error: %s", err)
		return nil, err
	}

	err = f.client.AddTag("ports", port.ID, fmt.Sprintf("%s:%s", VMTagPrefix, f.vmUUID))
	if err != nil {
		fmt.Errorf("failed to add tag to port:%s with error %w", port.ID, err)
		return nil, err
	}

	p := &PortResource{
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
	err = f.client.DeletePort(res.GetResourceId())
	if err != nil {
		return fmt.Errorf("failed to delete port with error: %w", err)
	}
	f.Unlock()
	return nil
}

func (f *PortFactory) GetClient() *neutron.Client {
	return f.client
}

func (f *PortFactory) GetConfig() (string, string) {
	return f.netID, f.subnetID
}

type PortResourceManager struct {
	factory *PortFactory
	pool    pool.ObjectPool
}

func NetConfFromPort(p *PortResource) ([]*rpc.NetConf, error) {
	var netConf []*rpc.NetConf

	port := p.port
	logger.Infof("############# neutron port value is %+v", port)
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
	subnet, err := client.GetSubnet(subnetId)
	if err != nil {
		return nil, fmt.Errorf("failed to get subnet with error: %w", err)
	}
	logger.Infof("********SubNet is: %+v ******", subnet)

	factory := &PortFactory{
		client:    client,
		netID:     netId,
		subnetID:  subnetId,
		nodeName:  config.Node.Name,
		vmUUID:    config.Node.UUID,
		projectID: config.Node.ProjectID,
		ports:     []*PortResource{},
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
			f := neutron.ListFilter{
				NetworkID:   netId,
				DeviceOwner: DeviceOwner,
				Tags:        fmt.Sprintf("%s:%s", VMTagPrefix, config.Node.UUID),
			}
			ports, err := client.ListPortWithFilter(f)
			if err != nil {
				return fmt.Errorf("failed to list ports allocated by this node %s with error: %w", config.Node.Name, err)
			}

			// loop ports to initialize pool inUse and idle
			for _, np := range ports {
				logger.Infof("MMMMMMMMMMMMMM port from neutron is %+v", np)
				pod, ok := portsMapping[np.ID]
				p := &PortResource{
					port: client.ConvertPort(subnet, np),
				}
				logger.Infof("MMMMMMMMMMMMMM After convert port is %+v", p.port)

				// update ports list in factory
				factory.ports = append(factory.ports, p)

				if ok {
					logger.Infof("** port %s in using by pod %s, add it into insue", np.ID, pod)
					holder.AddInuse(p)
				} else {
					logger.Infof("!!!!! port %s is not using by any pod add it into idle", np.ID)
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
		factory: factory,
		pool:    pool,
	}

	return mgr, nil
}

func (m *PortResourceManager) Allocate(ctx *ResourceContext, resId string) (types.NetworkResource, error) {
	// if ip address specified
	if requireStaticIP(ctx) {
		logger.Infof("Allocate Static IP adresses for pod: %s", ctx.PodInfo.PodInfoKey())
		return m.acquireStaticAddress(ctx)
	}
	return m.pool.Acquire(ctx.Context, resId)
}

func (m *PortResourceManager) Release(ctx *ResourceContext, resId string) error {
	if ctx != nil && ctx.PodInfo != nil {
		logger.Infof("@@@@@@@@@@@ POd is %s, stick time is %s", ctx.PodInfo.PodInfoKey(), ctx.PodInfo.IpStickTime)
		return m.pool.ReleaseWithReverse(resId, ctx.PodInfo.IpStickTime)
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

func requireStaticIP(ctx *ResourceContext) bool {
	annotations := ctx.Pod.Annotations
	return len(annotations[IpAddressAnnotation]) > 0 || len(annotations[IpPoolAnnotation]) > 0
}

func (m *PortResourceManager) acquireStaticAddress(ctx *ResourceContext) (types.NetworkResource, error) {
	ipAddress := ctx.Pod.Annotations[IpAddressAnnotation]

	filter := neutron.ListFilter{
		NetworkID:   m.factory.netID,
		DeviceOwner: DeviceOwner,
	}

	if len(ipAddress) > 0 {
		ip := net.ParseIP(ipAddress)
		if ip == nil {
			return nil, fmt.Errorf("failed to parse ip with annotation %s", ipAddress)
		}

		//get all allocated ports
		ports, err := m.factory.client.ListPortWithFilter(filter)
		if err != nil {
			return nil, fmt.Errorf("failed to list ports allocated by rubble with error: %w", err)
		}

		occupied := false
		// if ip existing return port else create new port with ip address
		subnet, err := m.factory.client.GetSubnet(m.factory.subnetID)
		if err != nil {
			return nil, fmt.Errorf("failed to get subnet with error: %w", err)
		}
		logger.Infof("********SubNet is: %+v ******", subnet)
		for _, p := range ports {
			port := m.factory.client.ConvertPort(subnet, p)
			if port.IP == ipAddress {
				logger.Infof("IP address %s is occupied by port %+v", ipAddress, p)
				occupied = true
				break
			}
		}

		if occupied {
			for _, item := range m.pool.GetIdle() {
				if item.GetResource().GetIPAddress() == ipAddress {
					logger.Infof("VVVVVV If occupied by by idel item %+v", item.GetResource())
					return m.pool.Acquire(ctx.Context, item.GetResource().GetResourceId())
				}
			}
			return nil, fmt.Errorf("IP address %s is occupied by port but not in pool idle queue", ipAddress)
		} else {
			// create port with specified ip address
			res, err := m.factory.Create(ipAddress)
			if err != nil {
				logger.Errorf("error create port with ip address %s, with error: %+v", ipAddress, err)
			} else {
				logger.Infof("add resource %s to pool idle", res.GetResourceId())
				// add to idle and acquire
				m.pool.AddIdle(res)
				return m.pool.Acquire(ctx.Context, res.GetResourceId())
			}
		}
	}

	return nil, fmt.Errorf("IP address %s is not valid", ipAddress)
}
