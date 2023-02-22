package utils

import "time"

const (
	DefaultCniTimeout   = 20 * time.Second
	DefaultSocketPath = "/var/run/cni/rubble.socket"
	DefaultCNIPath = "/opt/cni/bin"
)
