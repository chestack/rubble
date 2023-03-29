package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rubble/pkg/plugin"
	"github.com/rubble/pkg/rpc"
	"github.com/rubble/pkg/utils"
	"google.golang.org/grpc"
	"net"
	"runtime"
	"strings"

	"github.com/rubble/pkg/log"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
)

var cniLog = log.DefaultLogger.WithField("component:", "rubble cni plugin")
var ipVlan = plugin.NewIPVlanDriver()
var ptp = plugin.NewPTPDriver()

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, "easystack vpc cni plugin")
}

func cmdAdd(args *skel.CmdArgs) error {
	log.SetLogOutput(utils.DefaultCNILogPath)
	cniLog.Debugf("*********** rubble cni do Add ******")

	addArgs, err := getCmdArgs(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultCniTimeout)
	defer cancel()
	client, conn, err := getRubbleClient(ctx)
	if err != nil {
		return fmt.Errorf("error create grpc client, %w", err)
	}
	defer conn.Close()

	result, err := doCmdAdd(ctx, client, &addArgs)
	if err != nil {
		cniLog.WithError(err).Error("error adding")
		return err
	}

	result.CNIVersion = addArgs.CNIVersion
	return types.PrintResult(result, addArgs.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	cniLog.Debugf("*********** rubble cni do Del ******")

	//1. call plugins to teardown all resources
	delArgs, err := getCmdArgs(args)
	if err != nil {
		return err
	}
	err = ptp.TearDown(&delArgs)
	if err != nil {
		return fmt.Errorf("failed to teardown ptp veth with error: %w", err)
	}

	err = ipVlan.TearDown(&delArgs)
	if err != nil {
		return fmt.Errorf("failed to teardown ipvlan device with error: %w", err)
	}

	//2. call rubble-daemon to release ip
	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultCniTimeout)
	defer cancel()
	client, conn, err := getRubbleClient(ctx)
	if err != nil {
		return fmt.Errorf("error create grpc client, %w", err)
	}
	defer conn.Close()

	reply, err := client.ReleaseIP(ctx, &rpc.ReleaseIPRequest{
		K8SPodName:             delArgs.K8sPodName,
		K8SPodNamespace:        delArgs.K8sPodNameSpace,
		K8SPodInfraContainerId: delArgs.K8sInfraContainerID,
	})
	if err != nil {
		err = fmt.Errorf("cmdDel: error release ip %w", err)
		return err
	}
	if !reply.Success {
		err = fmt.Errorf("cmdDel: release ip return not success")
	}
	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func getCmdArgs(args *skel.CmdArgs) (utils.CniCmdArgs, error) {
	netConf, err := loadNetConf(args.StdinData)
	if err != nil {
		return utils.CniCmdArgs{}, err
	}

	k8sArgs, err := getK8sArgs(args)
	if err != nil {
		return utils.CniCmdArgs{}, err
	}

	cmdArgs := utils.CniCmdArgs{
		NetConf: netConf,
		NetNS:   args.Netns,
		K8sArgs: k8sArgs,
		RawArgs: args,
	}
	return cmdArgs, nil
}

func getRubbleClient(ctx context.Context) (rpc.RubbleBackendClient, *grpc.ClientConn, error) {
	conn, err := grpc.DialContext(ctx, utils.DefaultSocketPath, grpc.WithInsecure(), grpc.WithContextDialer(
		func(ctx context.Context, s string) (net.Conn, error) {
			unixAddr, err := net.ResolveUnixAddr("unix", utils.DefaultSocketPath)
			if err != nil {
				return nil, fmt.Errorf("error resolve addr, %w", err)
			}
			d := net.Dialer{}
			return d.DialContext(ctx, "unix", unixAddr.String())
		}))
	if err != nil {
		return nil, nil, fmt.Errorf("error dial to rubble server %s, with error: %w", utils.DefaultSocketPath, err)
	}

	client := rpc.NewRubbleBackendClient(conn)
	return client, conn, nil
}

func doCmdAdd(ctx context.Context, client rpc.RubbleBackendClient, cmdArgs *utils.CniCmdArgs) (*current.Result, error) {
	cniLog.Infof("Do add nic for pod: %s/%s.", cmdArgs.K8sPodNameSpace, cmdArgs.K8sPodName)
	cniLog.Infof("netConf is: %+v", cmdArgs.NetConf)
	cniLog.Infof("stdin from args is: %s", string(cmdArgs.RawArgs.StdinData))

	// 1. ipam with neutron
	allocResult, err := client.AllocateIP(ctx, &rpc.AllocateIPRequest{
		Netns:                  cmdArgs.NetNS,
		K8SPodName:             cmdArgs.K8sPodName,
		K8SPodNamespace:        cmdArgs.K8sPodNameSpace,
		K8SPodInfraContainerId: cmdArgs.K8sInfraContainerID,
		IfName:                 cmdArgs.RawArgs.IfName,
	})
	if err != nil {
		err = fmt.Errorf("cmdAdd: error allocate ip %w", err)
		return nil, err
	}
	if !allocResult.Success {
		err = fmt.Errorf("cmdAdd: allocate ip return not success")
	}

	cniLog.Infof("Allocate reply is %+v", allocResult)

	// 2.setup ipVlan interface eth0 in container
	// (TODO) convert allocResult to cni result
	tmpResult, err := ipVlan.Setup(cniLog, allocResult, cmdArgs)
	if err != nil {
		err = fmt.Errorf("failed to setup ipvlan device with error: %w", err)
		return nil, err
	}

	// 3.setup ptp veth in container because the ipVlan limitation: https://www.cni.dev/plugins/current/main/ipvlan/#notes
	result, err := ptp.Setup(cniLog, tmpResult, cmdArgs)
	if err != nil {
		err = fmt.Errorf("failed to setup ptp veth pair with error: %w", err)
		return nil, err
	}

	return result, nil
}

func loadNetConf(bytes []byte) (*utils.NetConf, error) {
	nc := &utils.NetConf{}
	if err := json.Unmarshal(bytes, nc); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	return nc, nil
}

func parseValueFromArgs(key, argString string) (string, error) {
	if argString == "" {
		return "", errors.New("CNI_ARGS is required")
	}
	args := strings.Split(argString, ";")
	for _, arg := range args {
		if strings.HasPrefix(arg, fmt.Sprintf("%s=", key)) {
			value := strings.TrimPrefix(arg, fmt.Sprintf("%s=", key))
			if len(value) > 0 {
				return value, nil
			}
		}
	}
	return "", fmt.Errorf("%s is required in CNI_ARGS", key)
}

func getK8sArgs(args *skel.CmdArgs) (*utils.K8sArgs, error) {

	podNamespace, err := parseValueFromArgs("K8S_POD_NAMESPACE", args.Args)
	if err != nil {
		return nil, err
	}

	podName, err := parseValueFromArgs("K8S_POD_NAME", args.Args)
	if err != nil {
		return nil, err
	}

	result := utils.K8sArgs{
		K8sPodName:          podName,
		K8sPodNameSpace:     podNamespace,
		K8sInfraContainerID: args.ContainerID,
	}
	return &result, nil
}
