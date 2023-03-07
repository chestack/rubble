package main

import (
	"flag"
	"github.com/rubble/pkg/utils"
	"os"

	"github.com/rubble/pkg/controller"
	"github.com/rubble/pkg/log"
)

var (
	logLevel        string
	daemonMode      string
	kubeConfig      string
	openstackConfig string

	neutronNet    string
	neutronSubnet string
)

func main() {
	fs := flag.NewFlagSet("rubble", flag.ExitOnError)

	fs.StringVar(&daemonMode, "daemon-mode", "vpc", "rubble network mode.")
	fs.StringVar(&logLevel, "log-level", "info", "rubble log level.")
	fs.StringVar(&kubeConfig, "kube-config", "", "Path to kube-config file.")
	fs.StringVar(&openstackConfig, "openstack-config", "", "Path to openstack config file.")
	fs.StringVar(&neutronNet, "neutron-network", "share_net", "network name or id")
	fs.StringVar(&neutronSubnet, "neutron-subnet", "share_net__subnet", "subnet name or id")
	err := fs.Parse(os.Args[1:])
	if err != nil {
		panic(err)
	}

	if err = controller.Run(utils.DefaultSocketPath, kubeConfig, openstackConfig, neutronNet, neutronSubnet); err != nil {
		log.DefaultLogger.Fatal(err)
	}
}
