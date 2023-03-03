package utils

import (
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"time"
)

const (
	DefaultCniTimeout = 20 * time.Second
	DefaultSocketPath = "/var/run/cni/rubble.socket"
	DefaultCNIPath    = "/opt/cni/bin"
	DefaultCNILogPath = "/var/log/rubble.cni.log"
)

type KubernetesArgs struct {
	K8sPodName          string
	K8sPodNameSpace     string
	K8sInfraContainerID string
}

type IPVlanArgs struct {
	Mode   string
	Master string
	MTU    int
}

type CniCmdArgs struct {
	NetConf    *types.NetConf
	NetNS      string
	K8sArgs    *KubernetesArgs
	InputArgs  *skel.CmdArgs
	IPVlanArgs *IPVlanArgs
}
