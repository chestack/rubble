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
