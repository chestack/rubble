## how to build

- build rubble cni-plugin: ```GOOS=linux go build cmd/cni/cni.go ```
- build rubble cni-server: ```GOOS=linux go build cmd/cni-server/cni-server.go```

## how to debug

use [cni/cnitool](https://github.com/containernetworking/cni/blob/main/cnitool/README.md) call rubble to simulate as containerd call rubble.

- start cni-server: ```./rubble-server --kube-config=/root/.kube/config```
- run cnitool to do cmdAdd: 
  ```
  export CNI_ARGS="IgnoreUnknown=1;K8S_POD_NAMESPACE=default;K8S_POD_NAME=nginx-5cdf8bbdf6-mfngn"
  export CNI_PATH=/opt/cni/bin
  
  ./cnitool add rubble /var/run/netns/testing
  
  tailf /var/log/rubble.cni.log
  ```


