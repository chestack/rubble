package utils

import (
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
)

type NetConf struct {
	types.NetConf
	Master       string `json:"master"`
	Mode         string `json:"mode"`
	MTU          int    `json:"mtu"`
	DefaultRoute bool   `json:"default_route"`
}

type K8sArgs struct {
	K8sPodName          string
	K8sPodNameSpace     string
	K8sInfraContainerID string
	K8sServiceCidr      string
}

type CniCmdArgs struct {
	*NetConf
	*K8sArgs
	RawArgs *skel.CmdArgs
	NetNS   string
}

type NodeInfo struct {
	UUID             string `json:"uuid"`
	Hostname         string `json:"hostname"`
	ProjectID        string `json:"project_id"`
	Name             string `json:"name"`
	AvailabilityZone string `json:availability_zone`
}

type DaemonConfigure struct {
	ServiceCIDR string `yaml:"service_cidr" json:"service_cidr"`
	NetID       string `yaml:"net_id" json:"net_id"`
	SubnetID    string `yaml:"subnet_id" json:"subnet_id"`
	MaxPoolSize int    `yaml:"max_pool_size" json:"max_pool_size"`
	MinPoolSize int    `yaml:"min_pool_size" json:"min_pool_size"`
	MaxIdleSize int    `yaml:"max_idle_size" json:"max_idle_size"`
	MinIdleSize int    `yaml:"min_idle_size" json:"min_idle_size"`
	Period      int    `yaml:"period" json:"period"`
	NodeName    string `yaml:"node_name" json:"node_name"`
	Node        *NodeInfo
}

type NetworkResource interface {
	GetResourceId() string
	GetType() string
	GetIPAddress() string
}
