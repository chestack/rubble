## [what && why vpc-cni](docs/why.md)

## how to build

- build cni binary plugin rubble : ```GOOS=linux go build cmd/cni/rubble.go ```
- build rubble daemon server: ```GOOS=linux go build cmd/cni-daemon/rubble-daemon.go```

## how to debug

use [cni/cnitool](https://github.com/containernetworking/cni/blob/main/cnitool/README.md) call rubble to simulate as containerd call rubble.

- start cni-server: 
  ```
  env OS_AUTH_URL=http://keystone-api.openstack.svc.cluster.local:80/v3 OS_DOMAIN_NAME=Default OS_PROJECT_NAME=service OS_USER_DOMAIN_NAME=Default OS_USERNAME=drone OS_PASSWORD=IcesNpQI ./rubble-daemon --kube-config=/root/.kube/config
  ```
  

- run cnitool to do cmdAdd:
```
  export CNI_ARGS="IgnoreUnknown=1;K8S_POD_NAMESPACE=default;K8S_POD_NAME=nginx-5cdf8bbdf6-mfngn"
  export CNI_PATH=/opt/cni/bin
  
  ./cnitool add rubble /var/run/netns/testing
  
  tailf /var/log/rubble.cni.log
  ```


