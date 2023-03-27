package neutron

import (
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
)

const (
	MetaDataURL = "http://169.254.169.254/openstack/latest/meta_data.json"
)

type Client struct {
	networkCliV2  *gophercloud.ServiceClient
	identityCliV3 *gophercloud.ServiceClient

	podsDeleteLock *sync.Mutex
	portIDs        map[string]string
}

func NewClient() (*Client, error) {
	provider, err := newProviderClientOrDie(false)
	if err != nil {
		return nil, err
	}
	domainTokenProvider, err := newProviderClientOrDie(true)
	if err != nil {
		return nil, err
	}

	netV2, err := newNetworkV2ClientOrDie(provider)
	if err != nil {
		return nil, err
	}

	idenV3, err := newIdentityV3ClientOrDie(domainTokenProvider)
	if err != nil {
		return nil, err
	}

	return &Client{
		networkCliV2:   netV2,
		identityCliV3:  idenV3,
		podsDeleteLock: &sync.Mutex{},
		portIDs:        make(map[string]string),
	}, nil
}

func newProviderClientOrDie(domainScope bool) (*gophercloud.ProviderClient, error) {
	opt, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		return nil, err
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
		return nil, err
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
	return p, nil
}

func newNetworkV2ClientOrDie(p *gophercloud.ProviderClient) (*gophercloud.ServiceClient, error) {
	cli, err := openstack.NewNetworkV2(p, gophercloud.EndpointOpts{})
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func newIdentityV3ClientOrDie(p *gophercloud.ProviderClient) (*gophercloud.ServiceClient, error) {
	cli, err := openstack.NewIdentityV3(p, gophercloud.EndpointOpts{})
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func (Client) GetVMMetadata() ([]byte, error) {
	resp, err := http.Get(MetaDataURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}
