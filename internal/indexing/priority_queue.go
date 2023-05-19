package indexing

import "container/heap"

type Item struct {
	Value    interface{}
	Priority int
	Index    int
}

// type PriorityQueue []*Item
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int {
	return len(pq)
}
func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].Priority < pq[j].Priority
}
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.Index = n
	*pq = append(*pq, item)
	heap.Fix(pq, item.Index)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) Peek() interface{} {
	old := *pq
	return old[0]
}

func NewPriorityQueue(max int) *PriorityQueue {
	pq := make(PriorityQueue, 0, max)
	heap.Init(&pq)
	return &pq
}
