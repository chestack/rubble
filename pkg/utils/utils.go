package utils

import (
	"io/ioutil"
	"math/rand"
	"regexp"
	"time"
)

const (
	DefaultCniTimeout   = 20 * time.Second
	DefaultSocketPath   = "/var/run/cni/rubble.socket"
	DefaultCNIPath      = "/opt/cni/bin"
	DefaultCNILogPath   = "/var/log/rubble.cni.log"
	DefaultIpVlanMode   = "l2"
	DefaultIpVlanMaster = "eth0"
	DefaultIpVlanRoute  = true
	DefaultDst          = "0.0.0.0/0"

	DefaultContainerVethName = "veth0"
	DefaultServiceCidr       = "10.222.0.0/16"

	DefaultDeamonConfigPath = "/etc/cni/rubble/rubble.json"

	ResourceTypePort = "portIp"

	DaemonDBPath = "/var/lib/cni/rubble/daemon.db"
	ResDBName = "PodPorts"

	charset = "abcdefghijklmnopqrstuvwxyz0123456789"
)

func IsValidUUID(uuid string) bool {
	r := regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$")
	return r.MatchString(uuid)
}

func GetIpVlanMaster(conf *NetConf) string {
	if len(conf.Master) > 0 {
		return conf.Master
	}
	return DefaultIpVlanMaster
}

func GetIpVlanMode(conf *NetConf) string {
	if len(conf.Mode) > 0 {
		return conf.Mode
	}
	return DefaultIpVlanMode
}

func GetIpVlanDefaultRoute(conf *NetConf) bool {
	return conf.DefaultRoute || DefaultIpVlanRoute
}

func GetServiceCidr(args *K8sArgs) string {
	if len(args.K8sServiceCidr) > 0 {
		return args.K8sServiceCidr
	}
	return DefaultServiceCidr
}

func RandomString(length int) string {
	rand.Seed(time.Now().UnixNano())

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func IfRuningOnVM() bool {
	_, err := ioutil.ReadFile("/proc/self/status")
	if err == nil {
		// If /proc exists, check if it contains a hypervisor file
		data, err := ioutil.ReadFile("/proc/cpuinfo")
		if err == nil && containsSubstring(string(data), "hypervisor") {
			return true
		} else {
			return false
		}
	}
	return false
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}