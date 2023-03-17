package ipam

import (
	"context"
	"sync"

	"github.com/rubble/pkg/k8s"
	types "github.com/rubble/pkg/utils"
)

type ResourceItem struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type PodResources struct {
	lock      sync.Mutex
	Resources []ResourceItem
	PodInfo   *k8s.PodInfo
}

type NetworkContext struct {
	Context   context.Context
	Resources []ResourceItem
	Pod       *k8s.PodInfo
}

func (p PodResources) GetResourceItemByType(resType string) []ResourceItem {
	p.lock.Lock()
	defer p.lock.Unlock()
	var ret []ResourceItem
	for _, r := range p.Resources {
		if resType == r.Type {
			ret = append(ret, ResourceItem{Type: resType, ID: r.ID})
		}
	}
	return ret
}

// ResourceManager Allocate/Release/Pool/Stick/GC pod resource
// managed pod and resource relationship
type ResourceManager interface {
	Allocate(context *NetworkContext, prefer string) (types.NetworkResource, error)
	Release(context *NetworkContext, resId string) error
	GarbageCollection(inUseResList map[string]interface{}, expireResList map[string]interface{}) error
}
