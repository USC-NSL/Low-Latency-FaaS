package controller

import (
	"sync"

	glog "github.com/golang/glog"
)

// Each worker has a pool to store instances which are waiting for the TID information sent through gRPC requests.
// |mutex| is required since both main thread and gRPC server thread may access it at the same time.
type InstanceStartupPool struct {
	pool  map[int]*Instance
	mutex sync.Mutex
}

func NewInstanceStartupPool() *InstanceStartupPool {
	return &InstanceStartupPool{
		pool: make(map[int]*Instance),
	}
}

// Adds an instance |ins| in the InstanceStartupPool |insPool|.
func (insPool *InstanceStartupPool) add(ins *Instance) {
	insPool.mutex.Lock()
	defer insPool.mutex.Unlock()

	if _, exist := insPool.pool[ins.port]; exist {
		glog.Errorf("Instance[%d]'s already existed in StartupPool", ins.port)
	}
	insPool.pool[ins.port] = ins
}

// Removes an instance (identified by its port) from the
// InstanceStartupPool pool after receiving its |tid|.
func (insPool *InstanceStartupPool) remove(port int, tid int) {
	insPool.mutex.Lock()
	defer insPool.mutex.Unlock()

	ins, exist := insPool.pool[port]
	if !exist {
		glog.Errorf("Try to remove Instance[%d], not found in StartupPool", port)
	}

	ins.notifyTid(tid)
	delete(insPool.pool, port)
}
