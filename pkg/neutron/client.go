package neutron

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"net/http"
	"os"
	"sync"
	"time"

	"k8s.io/klog"
)

const (
	NETWORK_ID            = "openstack.org/network_id"
	NETWORK_NAME          = "openstack.org/network_name"
	SUBNET_ID             = "openstack.org/subnet_id"
	SUBNET_NAME           = "openstack.org/subnet_name"
	PORT_ID               = "openstack.org/port_id"
	PORT_NAME             = "openstack.org/port_name"
)

type Client struct {
	networkCliV2  *gophercloud.ServiceClient
	identityCliV3 *gophercloud.ServiceClient

	podsDeleteLock *sync.Mutex
}

func NewClient() *Client {
	provider := newProviderClientOrDie(false)
	domainTokenProvider := newProviderClientOrDie(true)
	return &Client{
		networkCliV2:   newNetworkV2ClientOrDie(provider),
		identityCliV3:  newIdentityV3ClientOrDie(domainTokenProvider),
		podsDeleteLock: &sync.Mutex{},
	}
}

func newProviderClientOrDie(domainScope bool) *gophercloud.ProviderClient {
	opt, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		klog.Fatalf("openstack auth options from environment error: %v", err)
	}
	// with OS_PROJECT_NAME in env, AuthOptionsFromEnv return project scope token
	// which can not list projects, we need a domain scope token here
	if domainScope {
		opt.TenantName = ""
		opt.Scope = &gophercloud.AuthScope{
			DomainName: os.Getenv("OS_DOMAIN_NAME"),
		}
	}
	p, err := openstack.AuthenticatedClient(opt)
	if err != nil {
		klog.Fatalf("openstack authenticate client error: %v", err)
	}
	p.HTTPClient = http.Client{
		Transport: http.DefaultTransport,
		Timeout:   time.Second * 60,
	}
	p.ReauthFunc = func() error {
		newprov, err := openstack.AuthenticatedClient(opt)
		if err != nil {
			return err
		}
		p.CopyTokenFrom(newprov)
		return nil
	}
	return p
}

func newNetworkV2ClientOrDie(p *gophercloud.ProviderClient) *gophercloud.ServiceClient {
	cli, err := openstack.NewNetworkV2(p, gophercloud.EndpointOpts{})
	if err != nil {
		klog.Fatalf("new NetworkV2Client error : %v", err)
	}
	return cli
}

func newIdentityV3ClientOrDie(p *gophercloud.ProviderClient) *gophercloud.ServiceClient {
	cli, err := openstack.NewIdentityV3(p, gophercloud.EndpointOpts{})
	if err != nil {
		klog.Fatalf("new NewIdentityV3 error : %v", err)
	}
	return cli
}
