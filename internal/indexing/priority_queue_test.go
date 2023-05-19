package indexing_test

import (
	"container/heap"
	"testing"

	"github.com/TechMDW/indexing/internal/indexing"
)

func TestPriorityQueue(t *testing.T) {
	pq := indexing.NewPriorityQueue(10)

	for i := 10; i > 0; i-- {
		heap.Push(pq, &indexing.Item{
			Priority: i,
		})
	}

	expected := 1
	for pq.Len() > 0 {
		item := heap.Pop(pq).(*indexing.Item)
		if item.Priority != expected {
			t.Errorf("Expected priority %d, but got %d", expected, item.Priority)
		}
		expected++
	}
}

func TestPriorityQueueWithMoreItemsThanHeapSize(t *testing.T) {
	pq := indexing.NewPriorityQueue(10)

	for i := 10; i > 0; i-- {
		heap.Push(pq, &indexing.Item{
			Priority: i,
		})
	}

	for i := 20; i > 10; i-- {
		heap.Push(pq, &indexing.Item{
			Priority: i,
		})
	}

	expected := 1

	for pq.Len() > 0 {
		item := heap.Pop(pq).(*indexing.Item)
		if item.Priority != expected {
			t.Errorf("Expected priority %d, but got %d", expected, item.Priority)
		}
		expected++
	}
}
