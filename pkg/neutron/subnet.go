package neutron

import (
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/rubble/pkg/utils"
)

func (c Client) GetSubnet(id string) (*subnets.Subnet, error) {
	r := subnets.Get(c.networkCliV2, id)
	return r.Extract()
}

func (c Client) ListSubnetworks() ([]subnets.Subnet, error) {
	opts := subnets.ListOpts{}
	pages, _ := subnets.List(c.networkCliV2, opts).AllPages()
	allSubnets, _ := subnets.ExtractSubnets(pages)
	return allSubnets, nil
}

func (c Client) GetSubnetworkID(name string) (string, error) {

	if utils.IsValidUUID(name) {
		return name, nil
	}

	var id string
	subnets, err := c.ListSubnetworks()
	if err != nil {
		return "", err
	}
	for _, net := range subnets {
		if net.Name == name {
			id = net.ID
			break
		}
	}
	return id, nil
}

func (c Client) getSubnetAsync(id string) func() (*subnets.Subnet, error) {
	ch := make(chan *subnets.Subnet)
	chErr := make(chan error)

	go func() {
		sb, err := c.GetSubnet(id)
		if err != nil {
			chErr <- err
		} else {
			ch <- sb
		}
	}()

	return func() (*subnets.Subnet, error) {
		defer close(ch)
		defer close(chErr)

		select {
		case err := <-chErr:
			return nil, err
		case sb := <-ch:
			return sb, nil
		}
	}
}
