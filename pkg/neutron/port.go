package neutron

import (
	"errors"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/portsbinding"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/pagination"
)

const (
	DeviceOwner    = "compute:rubble"
	FipDeviceOwner = "network:kubernetes-pod"
)

type CreateOpts struct {
	Name          string
	NetworkID     string
	SubnetID      string
	IPAddress     string
	ProjectID     string
	SecurityGroup string
}

type Port struct {
	Name     string
	ID       string
	SubnetID string
	MAC      string
	IP       string
	CIDR     string
	Gateway  string
	MTU      int
	Sgs      []string
}

func (c Client) ListPortWithNetworkID(networkID string) ([]ports.Port, error) {
	var (
		opts   ports.ListOpts
		actual []ports.Port
		err    error
	)
	opts = ports.ListOpts{
		NetworkID: networkID,
	}
	err = ports.List(c.networkCliV2, opts).EachPage(func(page pagination.Page) (bool, error) {
		actual, err = ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}

		return true, nil
	})
	return actual, err
}

func (c Client) ListPortWithTag(networkID, tag string) ([]ports.Port, error) {
	var (
		opts   ports.ListOpts
		actual []ports.Port
		err    error
	)
	opts = ports.ListOpts{
		NetworkID: networkID,
		Tags:      tag,
	}
	err = ports.List(c.networkCliV2, opts).EachPage(func(page pagination.Page) (bool, error) {
		actual, err = ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}

		return true, nil
	})
	return actual, err
}

func (c Client) CreatePortWithFip(networkID, floatingip string) (*ports.Port, error) {
	type FixedIPOpt struct {
		SubnetID        string `json:"subnet_id,omitempty"`
		IPAddress       string `json:"ip_address,omitempty"`
		IPAddressSubstr string `json:"ip_address_subdir,omitempty"`
	}
	type FixedIPOpts []FixedIPOpt

	opts := ports.CreateOpts{
		NetworkID: networkID,
		FixedIPs: FixedIPOpts{
			{
				IPAddress: floatingip,
			},
		},
		DeviceOwner: FipDeviceOwner,
	}

	p, err := ports.Create(c.networkCliV2, opts).Extract()
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (c Client) DeletePortWithFip(networkID, floatingip string) error {
	var (
		opts   ports.ListOpts
		actual []ports.Port
		err    error
	)
	opts = ports.ListOpts{
		NetworkID: networkID,
	}
	err = ports.List(c.networkCliV2, opts).EachPage(func(page pagination.Page) (bool, error) {
		actual, err = ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}

		return true, nil
	})
	for _, p := range actual {
		for _, ip := range p.FixedIPs {
			if ip.IPAddress == floatingip {
				return ports.Delete(c.networkCliV2, p.ID).ExtractErr()
			}
		}
	}
	return errors.New("delete port failed, err: not found")
}

func (c Client) CreatePort(opts *CreateOpts) (Port, error) {

	type FixedIPOpt struct {
		SubnetID        string `json:"subnet_id,omitempty"`
		IPAddress       string `json:"ip_address,omitempty"`
		IPAddressSubstr string `json:"ip_address_subdir,omitempty"`
	}
	type FixedIPOpts []FixedIPOpt

	copts := ports.CreateOpts{
		Name:      opts.Name,
		NetworkID: opts.NetworkID,
		FixedIPs: FixedIPOpts{
			{
				SubnetID:  opts.SubnetID,
				IPAddress: opts.IPAddress,
			},
		},
		SecurityGroups: &[]string{},
	}

	/*	ss := strings.Split(opts.SecurityGroup, ",")
		if len(ss) > 0 {
			copts.SecurityGroups = &ss
		}*/

	sbRes := c.getSubnetAsync(opts.SubnetID)
	netRes := c.getNetworkAsync(opts.NetworkID)

	p, err := ports.Create(c.networkCliV2, copts).Extract()
	if err != nil {
		return Port{}, err
	}

	sb, err := sbRes()
	if err != nil {
		defer c.DeletePort(p.ID)
		return Port{}, err
	}

	_, mtu, err := netRes()
	if err != nil {
		defer c.DeletePort(p.ID)
		return Port{}, err
	}

	np := Port{
		Name:     p.Name,
		ID:       p.ID,
		SubnetID: opts.SubnetID,
		MAC:      p.MACAddress,
		IP:       p.FixedIPs[0].IPAddress,
		CIDR:     sb.CIDR,
		Gateway:  sb.GatewayIP,
		MTU:      mtu,
		Sgs:      p.SecurityGroups,
	}
	return np, nil
}

func (c Client) getPort(id string) (*ports.Port, error) {
	return ports.Get(c.networkCliV2, id).Extract()
}

func (c Client) DeletePort(id string) error {
	r := ports.Delete(c.networkCliV2, id)
	return r.ExtractErr()
}

func (c Client) RememberPortID(key, id string) {
	c.podsDeleteLock.Lock()
	defer c.podsDeleteLock.Unlock()
	c.portIDs[key] = id
}

func (c Client) ForgetPortID(key string) {
	c.podsDeleteLock.Lock()
	defer c.podsDeleteLock.Unlock()
	delete(c.portIDs, key)
}

func (c Client) PodToPortID(key string) (string, bool) {
	c.podsDeleteLock.Lock()
	defer c.podsDeleteLock.Unlock()
	id, ok := c.portIDs[key]
	return id, ok
}

// BindPort 将一个 Neutron Port 绑定到一个主机上。
// 主要用于 CNI 在配置 Pod 网卡的时候，要将对应 Port 绑定到
// hostID 所对应的主机上， ovs 才通
func (c Client) BindPortTOHost(id, hostID, deviceID string) error {
	updateOpts := portsbinding.UpdateOptsExt{
		UpdateOptsBuilder: ports.UpdateOpts{
			DeviceOwner: func(s string) *string { return &s }(DeviceOwner),
			DeviceID:    &deviceID,
		},
		HostID:   &hostID,
		VNICType: "normal",
	}

	_, err := ports.Update(c.networkCliV2, id, updateOpts).Extract()

	return err
}

// WaitPortActive 返回一个函数，调用该函数会阻塞指定的 Neutron Port 状态变成 ACTIVE,
// 或者超时返回错误
func (c Client) WaitPortActive(id string, timeout float64) error {
	start := time.Now()
	ch := make(chan error, 1)
	status := "EMPTY"

	go func() {
		tk := time.NewTicker(time.Duration(2) * time.Second)
		for range tk.C {
			elapse := time.Since(start)
			if elapse.Seconds() > timeout {
				ch <- fmt.Errorf("waiting port %s become ACTIVE timeout out: %.2f seconds", id, timeout)
				break
			}

			p, err := c.getPort(id)
			if err != nil {
				continue
			} else {
				if status != p.Status {
					status = p.Status
				}
				if p.Status == "ACTIVE" {
					ch <- nil
					break
				}
			}
		}
		tk.Stop()
	}()
	return <-ch
}
