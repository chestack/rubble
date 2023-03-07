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

var cniLog = log.DefaultLogger.WithField("component:", "rubble cni")
var ipvlan = plugin.NewIPVlanDriver()

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

	netConf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	k8sArgs, err := getK8sArgs(args)
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

	log.SetLogDebug()
	cniLog.Debugf("*********** rubble cni debug mode ******")

	addArgs := utils.CniCmdArgs{
		NetConf: netConf,
		NetNS:   args.Netns,
		K8sArgs: k8sArgs,
		RawArgs: args,
	}

	result, err := doCmdAdd(ctx, client, &addArgs)
	if err != nil {
		cniLog.WithError(err).Error("error adding")
		return err
	}

	result.CNIVersion = netConf.CNIVersion
	return types.PrintResult(result, netConf.CNIVersion)
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

	result, err := ipvlan.Setup(cniLog, allocResult, cmdArgs)
	if err != nil {
		err = fmt.Errorf("failed to setup ipvlan with error: %w", err)
		return nil, err
	}

	return result, nil
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
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
