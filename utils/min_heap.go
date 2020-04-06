package utils

type MinHeap []int

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h MinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h MinHeap) Contain(target int) bool {
	for i := 0; i < len(h); i++ {
		if h[i] == target {
			return true
		}
	}
	return false
}

// Push/Pop use pointer receivers because they modify the slice length.
// Adds x as element Len().
func (h *MinHeap) Push(x interface{}) {
	*h = append(*h, x.(int))
}

// Removes and returns element Len() - 1.
func (h *MinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
