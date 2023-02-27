package controller

import (
	"fmt"
	"github.com/rubble/pkg/rpc"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/rubble/pkg/log"
	"google.golang.org/grpc"
)

var logger = log.DefaultLogger.WithField("component:", "rubble cni-server")

func Run(socketFilePath, kubeConfig, openstackConfig string) error {

	if err := os.MkdirAll(filepath.Dir(socketFilePath), 0700); err != nil {
		return err
	}

	if err := syscall.Unlink(socketFilePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	l, err := net.Listen("unix", socketFilePath)
	if err != nil {
		return fmt.Errorf("error listen at %s: %v", socketFilePath, err)
	}

	grpcServer := grpc.NewServer()
	rubble, err := newRubbleService(kubeConfig, openstackConfig)
	if err != nil {
		return err
	}
	rpc.RegisterRubbleBackendServer(grpcServer, rubble)

	stop := make(chan struct{})

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigs
		logger.Infof("got system signal: %v, exiting", sig)
		stop <- struct{}{}
	}()

	logger.Infof("Starting rubble cni-server...")
	go func() {
		err = grpcServer.Serve(l)
		if err != nil {
			logger.Fatalf("error start grpc server: %v", err)
			stop <- struct{}{}
		}
	}()

	<-stop
	grpcServer.GracefulStop()
	return nil
}
