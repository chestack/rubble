package plugin

import (
	"fmt"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/rubble/pkg/utils"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"net"
)

type PTPDriver struct{}

func NewPTPDriver() *PTPDriver {
	return &PTPDriver{}
}

func getLinkIpAddrs(name string) (*net.IPNet, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get ip vlan master: %s, with error: %w", name, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses for interface: %s, with error: %w", name, err)
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet, nil
		}
	}
	return nil, fmt.Errorf("no ipaddress found for interface: %s", name)
}

func setupContainerVeth(logger *logrus.Entry, netns ns.NetNS, ifName string, args *utils.CniCmdArgs, pr *current.Result) (*current.Interface, *current.Interface, error) {
	nodeGw, err := getLinkIpAddrs(utils.GetIpVlanMaster(args.NetConf))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get node gateway with error: %w", err)
	}
	logger.Infof("########### IP address for interface is :%s", nodeGw.IP.String())

	hostInterface := &current.Interface{}
	containerInterface := &current.Interface{}

	err = netns.Do(func(hostNS ns.NetNS) error {
		hostVeth, contVeth0, err := ip.SetupVeth(ifName, args.MTU, "", hostNS)
		if err != nil {
			return err
		}
		hostInterface.Name = hostVeth.Name
		hostInterface.Mac = hostVeth.HardwareAddr.String()
		containerInterface.Name = contVeth0.Name
		containerInterface.Mac = contVeth0.HardwareAddr.String()
		containerInterface.Sandbox = netns.Path()

		contVeth, err := netlink.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to get link %q: %v", ifName, err)
		}
		err = netlink.LinkSetUp(contVeth)
		if err != nil {
			return fmt.Errorf("failed to set link %s up with error: %v", ifName, err)
		}

		svcCidr := utils.GetServiceCidr(args.K8sArgs)
		dst, _ := netlink.ParseIPNet(svcCidr)

		routes := []netlink.Route{
			{
				LinkIndex: contVeth.Attrs().Index,
				Dst: &net.IPNet{
					IP: nodeGw.IP,
					Mask: net.CIDRMask(32, 32),
				},
				Scope: netlink.SCOPE_LINK,
			},
			{
				LinkIndex: contVeth.Attrs().Index,
				Dst: dst,
				Gw: nodeGw.IP,
			},
		}

		for _, route := range routes {
			logger.Infof("########### Route is :%+v", route)
			if err := netlink.RouteAdd(&route); err != nil {
				return fmt.Errorf("failed to add route %+v: %w", route, err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, nil, err
	}
	return hostInterface, containerInterface, nil
}

func setupHostVeth(logger *logrus.Entry, vethName string, result *current.Result) error {
	hostVeth, err := netlink.LinkByName(vethName)
	if err != nil {
		return fmt.Errorf("failed to get link %q: %v", vethName, err)
	}

	route := netlink.Route{
		LinkIndex: hostVeth.Attrs().Index,
		Dst: &net.IPNet{
			IP: result.IPs[0].Address.IP,
			Mask: net.CIDRMask(32, 32),
		},
		Scope: netlink.SCOPE_LINK,
	}

	logger.Infof("########### Route is :%+v", route)
	if err = netlink.RouteAdd(&route); err != nil {
		return fmt.Errorf("failed to add route %+v on host with error: %w", route, err)
	}

	return nil
}

func (d *PTPDriver) Setup(logger *logrus.Entry, pre *current.Result, args *utils.CniCmdArgs) (*current.Result, error) {
	// Convert whatever the IPAM result was into the current Result type

	logger.Infof("######## Start PTP, current result is: %+v", *pre)
	result, _ := current.NewResultFromResult(pre)
	logger.Infof("######### After convert, result is: %+v", *result)

	if len(result.IPs) == 0 {
		return nil, fmt.Errorf("missing IP config")
	}

	netNs, err := ns.GetNS(args.NetNS)
	if err != nil {
		return nil, fmt.Errorf("failed to open netns %q: %v", args.NetNS, err)
	}
	defer netNs.Close()

	hostInterface, _, err := setupContainerVeth(logger, netNs, utils.DefaultContainerVethName, args, result)
	if err != nil {
		return nil, fmt.Errorf("failed to create veth with error: %w", err)
	}

	if err = setupHostVeth(logger, hostInterface.Name, result); err != nil {
		return nil, fmt.Errorf("failed to setup veth pair on host with error: %w", err)
	}

	return result, nil
}
