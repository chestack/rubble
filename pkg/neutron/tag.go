package neutron

import (
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/attributestags"
)

// AddTag create network from proton api
func (c Client) AddTag(resourceType, resourceID, tag string) error {
	return attributestags.Add(c.networkCliV2, resourceType, resourceID, tag).ExtractErr()
}
