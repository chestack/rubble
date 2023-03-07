package utils

import (
	"regexp"
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
