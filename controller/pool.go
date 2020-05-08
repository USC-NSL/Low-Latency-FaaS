package controller

import (
	"sync"

	glog "github.com/golang/glog"
)

// Each worker has a pool to store instances which are waiting for the TID information sent through gRPC requests.
// |mutex| is required since both main thread and gRPC server thread may access it at the same time.
type InstancePool struct {
	pool  map[int]*Instance
	mutex sync.Mutex
}

func NewInstancePool() *InstancePool {
	return &InstancePool{
		pool: make(map[int]*Instance),
	}
}

// Adds an instance |ins| in the InstancePool |insPool|.
func (insPool *InstancePool) add(ins *Instance) {
	insPool.mutex.Lock()
	defer insPool.mutex.Unlock()

	if _, exist := insPool.pool[ins.port]; exist {
		glog.Errorf("Instance[%d]'s already existed in StartupPool", ins.port)
	}
	insPool.pool[ins.port] = ins
}

// Returns an instance |ins| from the InstancePool |insPool|.
func (insPool *InstancePool) get(port int) *Instance {
	insPool.mutex.Lock()
	defer insPool.mutex.Unlock()

	ins, exist := insPool.pool[port]
	if !exist {
		glog.Errorf("Instance[%d] is not found in StartupPool", port)
		return nil
	}
	return ins
}

// Removes an instance, indexed by its port, from the
// InstancePool |insPool|.
func (insPool *InstancePool) remove(port int) {
	insPool.mutex.Lock()
	defer insPool.mutex.Unlock()

	_, exist := insPool.pool[port]
	if !exist {
		glog.Errorf("Try to remove Instance[%d], not found in StartupPool", port)
		return
	}

	delete(insPool.pool, port)
}

// Implements Len, Less and Swap for using "sort" package.
type SGroupSlice []*SGroup

func (pool SGroupSlice) Len() int {
	return len(pool)
}

func (pool SGroupSlice) Less(i, j int) bool {
	return pool[i].GetPktRate() < pool[j].GetPktRate()
}

func (pool SGroupSlice) Swap(i, j int) {
	pool[i], pool[j] = pool[j], pool[i]
}

// A thread-safe implementation of SGroupSlice
type SGroupPool struct {
	pool  SGroupSlice
	mutex sync.Mutex
}

func NewSGroupPool() *SGroupPool {
	return &SGroupPool{
		pool: make([]*SGroup, 0),
	}
}

// Adds a SGroup |newSG| into the SGroupPool |sgPool|.
func (sgPool *SGroupPool) add(newSG *SGroup) {
	sgPool.mutex.Lock()
	defer sgPool.mutex.Unlock()

	for _, sg := range sgPool.pool {
		if sg.ID() == newSG.ID() {
			glog.Errorf("SGroup[%d]'s already existed in SGroupPool", newSG.ID())
			return
		}
	}

	sgPool.pool = append(sgPool.pool, newSG)
}

// Returns a SGroup |sg| from the SGroupPool |sgPool|.
func (sgPool *SGroupPool) get(groupID int) *SGroup {
	sgPool.mutex.Lock()
	defer sgPool.mutex.Unlock()

	for _, sg := range sgPool.pool {
		if sg.ID() == groupID {
			return sg
		}
	}

	glog.Errorf("SGroup[%d] not found in SGroupPool", groupID)
	return nil
}

// Removes a SGroup (identified by its ID) from the SGroupPool |sgPool|.
func (sgPool *SGroupPool) remove(groupID int) {
	sgPool.mutex.Lock()
	defer sgPool.mutex.Unlock()

	for i, sg := range sgPool.pool {
		if sg.ID() == groupID {
			sgPool.pool = append(sgPool.pool[:i], sgPool.pool[i+1:]...)
			return
		}
	}

	glog.Errorf("Try to remove SGroup[%d], not found in SGroupPool", groupID)
}
