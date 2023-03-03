package neutron

import (
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
)

func (c Client) GetSubnet(id string) (*subnets.Subnet, error) {
	r := subnets.Get(c.networkCliV2, id)
	return r.Extract()
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
