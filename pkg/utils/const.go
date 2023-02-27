package utils

import (
	"time"
)

const (
	DefaultCniTimeout = 20 * time.Second
	DefaultSocketPath = "/var/run/cni/rubble.socket"
	DefaultCNIPath    = "/opt/cni/bin"
	DefaultCNILogPath = "/var/log/rubble.cni.log"
)

// K8SArgs is cni args of kubernetes
type K8SArgs struct {
	K8sPodName          string
	K8sPodNameSpace     string
	K8sInfraContainerID string
}
