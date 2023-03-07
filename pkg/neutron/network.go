package neutron

import (
	"fmt"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/mtu"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/rubble/pkg/utils"
)

func (c Client) getNetwork(id string) (*networks.Network, int, error) {
	var actual []struct {
		networks.Network
		mtu.NetworkMTUExt
	}

	allPages, err := networks.List(c.networkCliV2, networks.ListOpts{ID: id}).AllPages()
	if err != nil {
		return nil, 0, err
	}

	err = networks.ExtractNetworksInto(allPages, &actual)
	if err != nil {
		return nil, 0, err
	}

	var mTU int
	for _, n := range actual {
		if n.ID == id {
			mTU = n.MTU
			break
		}
	}
	if mTU == 0 {
		return nil, 0, fmt.Errorf("mtu not found for network %s", id)
	}

	r := networks.Get(c.networkCliV2, id)
	n, err := r.Extract()
	return n, mTU, err
}

func (c Client) getNetworkAsync(id string) func() (*networks.Network, int, error) {
	ch := make(chan *networks.Network)
	chErr := make(chan error)
	var MTU int

	go func() {
		var err error
		var sb *networks.Network
		sb, MTU, err = c.getNetwork(id)
		if err != nil {
			chErr <- err
		} else {
			ch <- sb
		}
	}()

	return func() (*networks.Network, int, error) {
		defer close(ch)
		defer close(chErr)

		select {
		case err := <-chErr:
			return nil, MTU, err
		case sb := <-ch:
			return sb, MTU, nil
		}
	}
}

func (c Client) GetNetwork(id string) (*networks.Network, error) {
	return networks.Get(c.networkCliV2, id).Extract()
}

func (c Client) ListNetworks() ([]networks.Network, error) {
	opts := networks.ListOpts{}
	pages, _ := networks.List(c.networkCliV2, opts).AllPages()
	allNetworks, _ := networks.ExtractNetworks(pages)
	return allNetworks, nil
}

func (c Client) GetNetworkID(name string) (string, error) {

	if utils.IsValidUUID(name) {
		return name, nil
	}

	var id string
	allNetworks, err := c.ListNetworks()
	if err != nil {
		return "", err
	}
	for _, network := range allNetworks {
		if network.Name == name {
			id = network.ID
			break
		}
	}
	return id, nil
}
