
package utils

import (
	"container/heap"
)

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

func (p *IndexPool) GetNextAvailable() int {
	if p.pool.Len() == 0 { return -1 }
	return heap.Pop(p.pool).(int)
}

func (p *IndexPool) Free(index int) {
	if !(*p.pool).Contain(index) {
		heap.Push(p.pool, index)
	}
}
