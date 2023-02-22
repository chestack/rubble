package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rubble/pkg/rpc"
	"github.com/rubble/pkg/utils"
	"google.golang.org/grpc"
	"net"
	"strings"

	"github.com/rubble/pkg/log"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/sirupsen/logrus"
)


var cniLog = log.DefaultLogger.WithField("component:", "rubble cni")

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, "easystack vpc cni plugin")
}

func cmdAdd(args *skel.CmdArgs) error {

	netConf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	podName, err := parseValueFromArgs("K8S_POD_NAME", args.Args)
	if err != nil {
		return err
	}
	podNamespace, err := parseValueFromArgs("K8S_POD_NAMESPACE", args.Args)
	if err != nil {
		return err
	}

	cniLog.WithFields(map[string]interface{}{
		"netns":        args.Netns,
		"podName":      podName,
		"podNamespace": podNamespace,
		"containerID":  args.ContainerID,
	}).Info("cmdAdd")

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultCniTimeout)
	defer cancel()

	client, conn, err := getRubbleClient(ctx)
	if err != nil {
		return fmt.Errorf("error create grpc client, %w", err)
	}
	defer conn.Close()

	cniLog.Infof("do add nics for pod: %s/%s", podNamespace, podName)
	response, err := doCmdAdd(ctx, cniLog, client, args, netConf)
	if err != nil {
		cniLog.WithError(err).Error("error adding")
		return err
	}

	result := generateCNIResult(netConf.CNIVersion, response, args)
	return types.PrintResult(&result, netConf.CNIVersion)
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

func doCmdAdd(ctx context.Context, logger *logrus.Entry, client rpc.RubbleBackendClient, args *skel.CmdArgs, netConf *types.NetConf) (*rpc.AllocateIPReply, error) {
	logger.Infof("args is: %+v", args)
	logger.Infof("netConf is: %+v", netConf)

	allocResult, err := client.AllocateIP(ctx, &rpc.AllocateIPRequest{
		Netns:                  args.Netns,
		K8SPodName:             "",
		K8SPodNamespace:        "",
		K8SPodInfraContainerId: "",
		IfName:                 args.IfName,
	})
	if err != nil {
		err = fmt.Errorf("cmdAdd: error allocate ip %w", err)
		return nil, err
	}
	if !allocResult.Success {
		err = fmt.Errorf("cmdAdd: allocate ip return not success")
	}
	return allocResult, nil
}

func generateCNIResult(cniVersion string, allocateResult *rpc.AllocateIPReply, args *skel.CmdArgs) current.Result {
	result := current.Result{
		CNIVersion: cniVersion,
	}

	result.Interfaces = append(result.Interfaces, &current.Interface{
		Name:    args.IfName,
		Sandbox: args.ContainerID,
	})

	return result
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
}

func loadNetConf(bytes []byte) (*types.NetConf, error) {
	nc := &types.NetConf{}
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