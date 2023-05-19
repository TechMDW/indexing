package main

import (
	"container/heap"
	"fmt"

	"github.com/TechMDW/indexing/internal/indexing"
)

func main() {
	heapSize := 7
	pq := indexing.NewPriorityQueue(heapSize)

	for i := 0; i < 10; i++ {
		item := &indexing.Item{
			Priority: i,
		}

		heap.Push(pq, item)
		if pq.Len() > heapSize {
			heap.Pop(pq)
		}
	}

	for i := 20; i < 23; i++ {
		item := &indexing.Item{
			Priority: i,
		}

		heap.Push(pq, item)
		if pq.Len() > heapSize {
			heap.Pop(pq)
		}
	}

	for pq.Len() > 0 {
		item := heap.Pop(pq).(*indexing.Item)
		fmt.Println(item.Priority)
	}
}
