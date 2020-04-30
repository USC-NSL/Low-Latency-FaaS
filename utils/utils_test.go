package utils

import (
	"os"
	"sync"
	"sort"
	"time"
	"reflect"
	"testing"
)

func TestMain(m *testing.M) {
	ret := m.Run()
	os.Exit(ret)
}

func TestMinHeapContain(t *testing.T) {
	heap := NewMinHeap()

	input1 := []int{1, 3, 5, 7, 9}
	for _, num := range input1 {
		heap.Push(num)
	}

	if heap.Size() != len(input1) {
		t.Errorf("Failed to push all numbers into the heap")
	}

	nums1 := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	truth1 := []bool{false, true, false, true, false, true, false, true, false, true, false}

	for i, num := range nums1 {
		if heap.Contain(num) != truth1[i] {
			t.Fatalf("Failed to tell %d is in MinHeap or not", num)
		}
	}
}

func TestMinHeapPushPop(t *testing.T) {
	heap := NewMinHeap()

	input1 := []int{10, 8, 6, 4, 2, 0, 9, 7, 5, 3, 1}
	truth1 := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, num := range input1 {
		heap.Push(num)
	}

	if heap.Size() != len(input1) {
		t.Errorf("Failed to push all numbers into the heap")
	}

	res1 := make([]int, 0)
	for heap.Size() > 0 {
		res1 = append(res1, heap.Pop().(int))
	}

	if reflect.DeepEqual(res1, truth1) {
		t.Errorf("Failed to pop all numbers in the sorted order")
	}
}

func TestIndexPoolSingleThread(t *testing.T) {
	base := 100
	numCount := 10000
	pool := NewIndexPool(base, numCount)

	for i := 0; i < numCount / 2; i++ {
		if pool.GetNextAvailable() != 100 + i {
			t.Errorf("Failed to get %d in the correct order", 100 + i)
		}
	}

	if pool.Size() != numCount / 2 {
		t.Errorf("Failed to pop enough numbers")
	}

	for i := 0; i < numCount / 2; i++ {
		if pool.GetNextAvailable() != base + numCount / 2 + i {
			t.Errorf("Failed to get %d in the correct order", 600 + i)
		}
	}

	if pool.Size() != 0 {
		t.Errorf("Failed to pop enough numbers")
	}

	for i := 0; i < numCount / 2; i++ {
		if pool.GetNextAvailable() != -1 {
			t.Errorf("Failed to return -1 when IndexPool is empty")
		}
	}
}

func TestIndexPoolMultiThread(t *testing.T) {
	base := 100
	numCount := 10000
	pool := NewIndexPool(base, numCount)
	var mu = &sync.Mutex{}
	nums := []int{}

	for i := 0; i < numCount / 2; i++ {
		go func() {
			num := pool.GetNextAvailable()
			mu.Lock()
			defer mu.Unlock()

			nums = append(nums, num)
		}()
	}
	time.Sleep(500 * time.Millisecond)

	if pool.Size() != numCount / 2 {
		t.Errorf("Failed to pop enough numbers")
	}

	for i := 0; i < numCount / 2; i++ {
		go func() {
			num := pool.GetNextAvailable()
			mu.Lock()
			defer mu.Unlock()

			nums = append(nums, num)
		}()
	}
	time.Sleep(500 * time.Millisecond)

	if pool.Size() != 0 || len(nums) != numCount {
		t.Errorf("Failed to pop enough numbers")
	}

	sort.Ints(nums)
	for i, num := range nums {
		if num != base + i {
			t.Errorf("Failed to pop %d. Poped %d", base + i, num)
		}
	}

	for i := 0; i < numCount / 2; i++ {
		go func() {
			if pool.GetNextAvailable() != -1 {
				t.Errorf("Failed to return -1 when IndexPool is empty")
			}
		}()
	}
}
