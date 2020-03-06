
package utils

import (
	"container/heap"
)

// IndexPool is a collection of numbers that are kept ready to use.
type IndexPool struct {
	pool 	*MinHeap
}

// Create a index pool between range [indexBase, indexBase + indexCount).
func NewIndexPool(indexBase int, indexCount int) (*IndexPool) {
	p := IndexPool{
		pool : &MinHeap{},
	}
	heap.Init(p.pool)

	for i := indexBase; i < indexBase + indexCount; i++ {
		heap.Push(p.pool, i)
	}
	return &p
}

// Fetch a number from the pool.
func (p *IndexPool) GetNextAvailable() int {
	if p.pool.Len() == 0 { return -1 }
	return heap.Pop(p.pool).(int)
}

// Free a number to the pool.
func (p *IndexPool) Free(index int) {
	if !(*p.pool).Contain(index) {
		heap.Push(p.pool, index)
	}
}
