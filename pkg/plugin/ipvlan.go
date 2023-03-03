package plugin

import (
	"fmt"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/rubble/pkg/rpc"
	"github.com/rubble/pkg/utils"
	"net"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type IPVlanDriver struct{}

func NewIPVlanDriver() *IPVlanDriver {
	return &IPVlanDriver{}
}

func (d *IPVlanDriver) Setup(logger *logrus.Entry, allocateResult *rpc.AllocateIPReply, args *utils.CniCmdArgs) (*current.Result, error) {
	netNs, err := ns.GetNS(args.NetNS)

	if err != nil {
		return nil, fmt.Errorf("failed to open netns %q: %v", args.NetNS, err)
	}
	defer netNs.Close()

	ipVlanSlave, err := createIPVlan(args, netNs)
	if err != nil {
		return nil, err
	}

	ipaddr := allocateResult.NetConfs[0].BasicInfo.PodIP.IPv4
	gwaddr := allocateResult.NetConfs[0].BasicInfo.GatewayIP.IPv4
	cidr := allocateResult.NetConfs[0].BasicInfo.PodCIDR.IPv4

	ip := &current.IPConfig{
		Interface: current.Int(0),
		Address: net.IPNet{
			IP:   net.ParseIP(ipaddr),
			Mask: net.IPMask(net.ParseIP(cidr).To4()),
		},
		Gateway: net.ParseIP(gwaddr),
	}

	result := &current.Result{
		Interfaces: []*current.Interface{ipVlanSlave},
		IPs:        []*current.IPConfig{ip},
	}

	logger.Infof("*********** result is %+v", result)

	err = netNs.Do(func(_ ns.NetNS) error {
		return ipam.ConfigureIface(args.InputArgs.IfName, result)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to configure ip address for ipvlan interface with error: %w", err)
	}
	return result, nil
}

func (d *IPVlanDriver) Teardown(args *utils.CniCmdArgs) error {
	return nil
}

func modeFromString(s string) (netlink.IPVlanMode, error) {
	switch s {
	case "", "l2":
		return netlink.IPVLAN_MODE_L2, nil
	case "l3":
		return netlink.IPVLAN_MODE_L3, nil
	case "l3s":
		return netlink.IPVLAN_MODE_L3S, nil
	default:
		return 0, fmt.Errorf("unknown ipvlan mode: %q", s)
	}
}

func createIPVlan(args *utils.CniCmdArgs, netns ns.NetNS) (*current.Interface, error) {
	slave := &current.Interface{}

	mode, err := modeFromString(args.IPVlanArgs.Mode)
	if err != nil {
		return nil, err
	}

	m, err := netlink.LinkByName(args.IPVlanArgs.Master)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup master %q: %v", args.IPVlanArgs.Master, err)
	}

	// due to kernel bug we have to create with tmpname or it might
	// collide with the name on the host and error out
	tmpName, err := ip.RandomVethName()
	if err != nil {
		return nil, err
	}

	mv := &netlink.IPVlan{
		LinkAttrs: netlink.LinkAttrs{
			MTU:         args.IPVlanArgs.MTU,
			Name:        tmpName,
			ParentIndex: m.Attrs().Index,
			Namespace:   netlink.NsFd(int(netns.Fd())),
		},
		Mode: mode,
	}

	if err = netlink.LinkAdd(mv); err != nil {
		return nil, fmt.Errorf("failed to create ipvlan: %v", err)
	}

	err = netns.Do(func(_ ns.NetNS) error {
		err = ip.RenameLink(tmpName, args.InputArgs.IfName)
		if err != nil {
			return fmt.Errorf("failed to rename ipvlan to %q: %w", args.InputArgs.IfName, err)
		}
		slave.Name = args.InputArgs.IfName

		// Re-fetch ipvlan to get all properties/attributes
		contIPVlan, err := netlink.LinkByName(slave.Name)
		if err != nil {
			return fmt.Errorf("failed to refetch ipvlan %q: %w", slave.Name, err)
		}
		slave.Mac = contIPVlan.Attrs().HardwareAddr.String()
		slave.Sandbox = netns.Path()

		return nil
	})
	if err != nil {
		return nil, err
	}

	return slave, nil
}