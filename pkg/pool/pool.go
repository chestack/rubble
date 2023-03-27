package pool

import (
	"context"
	"errors"
	"fmt"
	"github.com/rubble/pkg/log"
	types "github.com/rubble/pkg/utils"
	"strings"
	"sync"
	"time"
)

var (
	ErrNoAvailableResource = errors.New("no available resource")
	ErrInvalidState        = errors.New("invalid state")
	ErrNotFound            = errors.New("not found")
	ErrContextDone         = errors.New("context done")
	ErrInvalidArguments    = errors.New("invalid arguments")
)

var logger = log.DefaultLogger.WithField("component:", "rubble pool")

const (
	CheckIdleInterval  = 1 * time.Minute
	defaultPoolBackoff = 1 * time.Minute
	DefaultMaxIdle     = 20
	DefaultCapacity    = 50
)

type ObjectPool interface {
	Acquire(ctx context.Context, resId string) (types.NetworkResource, error)
	ReleaseWithReverse(resId string, reverse time.Duration) error
	Release(resId string) error
	AcquireAny(ctx context.Context) (types.NetworkResource, error)
	Stat(resId string) error
	GetInUse() map[string]types.NetworkResource
	GetIdle() []*poolItem
	AddIdle(res types.NetworkResource)
}

type ResourceHolder interface {
	AddIdle(resource types.NetworkResource)
	AddInuse(resource types.NetworkResource)
}

type ObjectFactory interface {
	//Create (TODO) gophercloud does not support creating multiple ports one API call
	Create(ip string) (types.NetworkResource, error)
	Dispose(types.NetworkResource) error
}

type SimpleObjectPool struct {
	inuse      map[string]types.NetworkResource
	idle       *Queue
	lock       sync.Mutex
	factory    ObjectFactory
	maxIdle    int
	minIdle    int
	capacity   int
	maxBackoff time.Duration
	notifyCh   chan interface{}
	// concurrency to create resource. tokenCh = capacity - (idle + inuse + dispose)
	tokenCh chan struct{}
}

type PoolConfig struct {
	Factory     ObjectFactory
	Initializer Initializer
	MinIdle     int
	MaxIdle     int
	MaxPoolSize int
	MinPoolSize int
	Capacity    int
}

type poolItem struct {
	res     types.NetworkResource
	reverse time.Time
}

func (i *poolItem) lessThan(other *poolItem) bool {
	return i.reverse.Before(other.reverse)
}

func (i *poolItem) GetResource() types.NetworkResource {
	return i.res
}

type Initializer func(holder ResourceHolder) error

func NewSimpleObjectPool(cfg PoolConfig) (ObjectPool, error) {
	if cfg.MinIdle > cfg.MaxIdle {
		return nil, ErrInvalidArguments
	}

	if cfg.MaxIdle > cfg.Capacity {
		return nil, ErrInvalidArguments
	}

	if cfg.MaxIdle == 0 {
		cfg.MaxIdle = DefaultMaxIdle
	}
	if cfg.Capacity == 0 {
		cfg.Capacity = DefaultCapacity
	}

	pool := &SimpleObjectPool{
		factory:  cfg.Factory,
		inuse:    make(map[string]types.NetworkResource),
		idle:     NewQueue(),
		maxIdle:  cfg.MaxIdle,
		minIdle:  cfg.MinIdle,
		capacity: cfg.Capacity,
		notifyCh: make(chan interface{}),
		tokenCh:  make(chan struct{}, cfg.Capacity),
	}

	if cfg.Initializer != nil {
		if err := cfg.Initializer(pool); err != nil {
			return nil, err
		}
	}

	if err := pool.preload(); err != nil {
		return nil, err
	}

	logger.Infof("pool initial state, capacity %d, maxIdle: %d, minIdle %d, idle: %s, inuse: %s",
		pool.capacity,
		pool.maxIdle,
		pool.minIdle,
		queueKeys(pool.idle),
		mapKeys(pool.inuse))

	go pool.startCheckIdleTicker()

	return pool, nil
}

func (p *SimpleObjectPool) startCheckIdleTicker() {
	p.checkIdle()
	ticker := time.NewTicker(CheckIdleInterval)
	for {
		select {
		case <-ticker.C:
			p.checkIdle()
			p.checkInsufficient()
		case <-p.notifyCh:
			p.checkIdle()
			p.checkInsufficient()
		}
	}
}

func mapKeys(m map[string]types.NetworkResource) string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

func queueKeys(q *Queue) string {
	var keys []string
	for i := 0; i < q.size; i++ {
		keys = append(keys, q.slots[i].res.GetResourceId())
	}
	return strings.Join(keys, ", ")
}

func (p *SimpleObjectPool) dispose(res types.NetworkResource) {
	logger.Infof("try dispose res %+v", res)
	if err := p.factory.Dispose(res); err != nil {
		//put it back on dispose fail
		logger.Warnf("failed dispose %s: %v, put it back to idle", res.GetResourceId(), err)
	} else {
		p.tokenCh <- struct{}{}
	}
}

func (p *SimpleObjectPool) tooManyIdle() bool {
	logger.Infof("Check Idle, idle size:%d, maxIdel:%d, pool size: %d, capacity:%d", p.idle.Size(), p.maxIdle, p.size(), p.capacity)
	return p.idle.Size() > p.maxIdle || (p.idle.Size() > 0 && p.size() > p.capacity)
}

//found resources that can be disposed, put them into dispose channel
//must in lock
func (p *SimpleObjectPool) checkIdle() {
	for p.tooManyIdle() {
		p.lock.Lock()
		item := p.idle.Peek()
		if item == nil {
			//impossible
			break
		}
		if item.reverse.After(time.Now()) {
			logger.Infof("NONONONO, will never after now")
			break
		}
		item = p.idle.Pop()
		p.lock.Unlock()
		res := item.res
		logger.Infof("try dispose res %+v", res)
		err := p.factory.Dispose(res)
		if err == nil {
			p.tokenCh <- struct{}{}
		} else {
			logger.Warnf("error dispose res: %+v", err)
			p.AddIdle(res)
		}
	}
}

func (p *SimpleObjectPool) checkInsufficient() {
	addition := p.needAddition()
	logger.Infof("Insufficient check...... addition is %d, idle size is %d, min idle is %d, in use is %d", addition, p.idle.Size(), p.minIdle, len(p.inuse))
	if addition <= 0 {
		return
	}
	var tokenAcquired int
	for i := 0; i < addition; i++ {
		// pending resources
		select {
		case <-p.tokenCh:
			tokenAcquired++
		default:
			continue
		}
	}
	logger.Infof("token acquired count: %v", tokenAcquired)
	if tokenAcquired <= 0 {
		return
	}

	var err error
	leftCount := 0
	for i := 0; i < tokenAcquired; i++ {
		logger.Infof("@@@@@@@@@@@@@@@@@@@@  Insufficient to create port")
		res, err := p.factory.Create("")
		if err != nil {
			logger.Errorf("error add idle network resources: %v", err)
			// release token
			p.tokenCh <- struct{}{}
			leftCount++
		} else {
			logger.Infof("add resource %s to pool idle", res.GetResourceId())
			p.AddIdle(res)
		}
	}

	if leftCount > 0 {
		logger.Infof("token acquired left: %d, err: %v", leftCount, err)
		p.notify()
	}
}

func (p *SimpleObjectPool) preload() error {
	for {
		// init resource sequential to avoid huge creating request on startup
		if p.idle.Size() >= p.minIdle {
			logger.Infof("@@@@@@@@@@@@ pool idle size > min Idle, %d > %d", p.idle.Size(), p.minIdle)
			break
		}

		if p.size() >= p.capacity {
			logger.Infof("@@@@@@@@@@@@ pool  size > capacity, %d > %d", p.size(), p.capacity)
			break
		}

		logger.Infof("@@@@@@@@@@@@ create port in preload")
		res, err := p.factory.Create("")
		if err != nil {
			return err
		}
		logger.Infof("@@@@@@@@@@@@ add %s into idle", res.GetResourceId())
		p.AddIdle(res)
	}

	tokenCount := p.capacity - p.size()
	logger.Infof("@@@@@@@@@@@@ token count is %d", tokenCount)
	for i := 0; i < tokenCount; i++ {
		p.tokenCh <- struct{}{}
	}

	return nil
}

func (p *SimpleObjectPool) sizeLocked() int {
	return p.idle.Size() + len(p.inuse)
}

func (p *SimpleObjectPool) needAddition() int {
	p.lock.Lock()
	defer p.lock.Unlock()
	addition := p.minIdle - p.idle.Size()
	if addition > (p.capacity - p.sizeLocked()) {
		return p.capacity - p.sizeLocked()
	}
	return addition
}

func (p *SimpleObjectPool) size() int {
	return p.idle.Size() + len(p.inuse)
}

func (p *SimpleObjectPool) getOneLocked(resId string) *poolItem {
	if len(resId) > 0 {
		item := p.idle.Rob(resId)
		if item != nil {
			return item
		}
	}
	return p.idle.Pop()
}

func (p *SimpleObjectPool) Acquire(ctx context.Context, resId string) (types.NetworkResource, error) {
	p.lock.Lock()
	//defer p.lock.Unlock()
	if p.idle.Size() > 0 {
		res := p.getOneLocked(resId).res
		p.inuse[res.GetResourceId()] = res
		p.lock.Unlock()
		logger.Infof("acquire (expect %s): return idle %s", resId, res.GetResourceId())
		return res, nil
	}
	size := p.size()
	if size >= p.capacity {
		p.lock.Unlock()
		logger.Infof("acquire (expect %s), size %d, capacity %d: return err %v", resId, size, p.capacity, ErrNoAvailableResource)
		return nil, ErrNoAvailableResource
	}

	p.lock.Unlock()

	select {
	case <-p.tokenCh:
		//should we pass ctx into factory.Create?
		res, err := p.factory.Create("")
		if err != nil {
			p.tokenCh <- struct{}{}
			return nil, fmt.Errorf("error create from factory: %v", err)
		}
		logger.Infof("acquire (expect %s): return newly %s", resId, res.GetResourceId())
		p.AddInuse(res)
		return res, nil
	case <-ctx.Done():
		logger.Infof("acquire (expect %s): return err %v", resId, ErrContextDone)
		return nil, ErrContextDone
	}
}

func (p *SimpleObjectPool) AcquireAny(ctx context.Context) (types.NetworkResource, error) {
	return p.Acquire(ctx, "")
}

func (p *SimpleObjectPool) Stat(resId string) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	_, ok := p.inuse[resId]
	if ok {
		return nil
	}

	if p.idle.Find(resId) != nil {
		return nil
	}

	return ErrNotFound
}

func (p *SimpleObjectPool) notify() {
	select {
	case p.notifyCh <- true:
	default:
	}
}

func (p *SimpleObjectPool) ReleaseWithReverse(resId string, reverse time.Duration) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	res, ok := p.inuse[resId]
	if !ok {
		logger.Infof("release %s: return err %v", resId, ErrInvalidState)
		return ErrInvalidState
	}

	logger.Infof("release %s, reverse %v: return success", resId, reverse)
	delete(p.inuse, resId)
	reverseTo := time.Now()
	if reverse > 0 {
		reverseTo = reverseTo.Add(reverse)
	}
	p.idle.Push(&poolItem{res: res, reverse: reverseTo})
	p.notify()
	return nil
}
func (p *SimpleObjectPool) Release(resId string) error {
	return p.ReleaseWithReverse(resId, time.Duration(0))
}

func (p *SimpleObjectPool) AddIdle(resource types.NetworkResource) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.idle.Push(&poolItem{res: resource, reverse: time.Now()})
}

func (p *SimpleObjectPool) AddInuse(res types.NetworkResource) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.inuse[res.GetResourceId()] = res
}

func (p *SimpleObjectPool) GetInUse() map[string]types.NetworkResource {
	return p.inuse
}

func (p *SimpleObjectPool) GetIdle() []*poolItem {
	return p.idle.slots
}
