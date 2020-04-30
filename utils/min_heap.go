package utils

type MinHeap struct {
	nums []int
}

func NewMinHeap() *MinHeap {
	return &MinHeap{
		nums: make([]int, 0),
	}
}

func (h MinHeap) Len() int           { return len(h.nums) }
func (h MinHeap) Size() int          { return len(h.nums) }
func (h MinHeap) Less(i, j int) bool { return h.nums[i] < h.nums[j] }
func (h MinHeap) Swap(i, j int)      { h.nums[i], h.nums[j] = h.nums[j], h.nums[i] }

func (h MinHeap) Contain(t int) bool {
	for i := 0; i < h.Len(); i++ {
		if h.nums[i] == t {
			return true
		}
	}
	return false
}

// Push/Pop use pointer receivers because they modify the slice length.
// Adds x as element Len().
func (h *MinHeap) Push(x interface{}) {
	h.nums = append(h.nums, x.(int))
}

// Removes and returns element Len() - 1.
func (h *MinHeap) Pop() interface{} {
	old := h.nums
	n := len(old)
	x := old[n-1]
	h.nums = old[0:(n - 1)]
	return x
}
