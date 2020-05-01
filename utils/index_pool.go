package utils

import (
	"container/heap"
	"sync"
)

// |IndexPool| manages a set of numbers. In many cases, resources
// are uniquely indexed by numbers. |IndexPool| is an abstraction
// that manages these resources in a multithread-safe way.
type IndexPool struct {
	pool  *MinHeap
	mutex sync.Mutex
}

// Create a index pool between range [indexBase, indexBase + indexCount).
func NewIndexPool(indexBase int, indexCount int) *IndexPool {
	p := IndexPool{
		pool: &MinHeap{},
	}
	heap.Init(p.pool)

	for i := indexBase; i < indexBase+indexCount; i++ {
		heap.Push(p.pool, i)
	}
	return &p
}

func (p *IndexPool) Size() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.pool.Len()
}

// Fetch a number from the pool.
func (p *IndexPool) GetNextAvailable() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.pool.Len() == 0 {
		return -1
	}
	return heap.Pop(p.pool).(int)
}

// Free a number to the pool.
func (p *IndexPool) Free(index int) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !(*p.pool).Contain(index) {
		heap.Push(p.pool, index)
	}
}
