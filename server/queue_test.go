// Manages pool of execution nodes
package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func cloneRequest(req *SimRequest) *SimRequest {
	return NewSimRequest(context.Background(), "1", req.Payload, req.IsHighPrio, req.IsFastTrack)
}

func fillQueue(t *testing.T, q *PrioQueue) {
	t.Helper()

	taskLowPrio := NewSimRequest(context.Background(), "1", []byte("taskLowPrio"), false, false)
	taskHighPrio := NewSimRequest(context.Background(), "1", []byte("taskHighPrio"), true, false)
	taskFastTrack := NewSimRequest(context.Background(), "1", []byte("tasFastTrack"), false, true)

	q.Push(taskLowPrio)
	q.Push(taskHighPrio)
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio))
	q.Push(cloneRequest(taskHighPrio)) // 11x highPrio
	q.Push(taskFastTrack)
	q.Push(cloneRequest(taskFastTrack))
	q.Push(cloneRequest(taskFastTrack))
	q.Push(cloneRequest(taskFastTrack))
	q.Push(cloneRequest(taskFastTrack)) // 5x fastTrack

	require.Equal(t, 5, len(q.fastTrack))
	require.Equal(t, 11, len(q.highPrio))
	require.Equal(t, 1, len(q.lowPrio))
}

func TestQueueBlockingPop(t *testing.T) {
	q := NewPrioQueue(0, 0, 0, 2, false)
	taskLowPrio := NewSimRequest(context.Background(), "1", []byte("taskLowPrio"), false, false)

	// Ensure queue.Pop is blocking
	t1 := time.Now()
	go func() { time.Sleep(100 * time.Millisecond); q.Push(taskLowPrio) }()
	resp := q.Pop()
	tX := time.Since(t1)
	require.NotNil(t, resp)
	require.True(t, tX >= 100*time.Millisecond)
}

func TestQueuePopping(t *testing.T) {
	// Test 1 - expected: fastTrack -> highPrio -> fastTrack -> highPrio
	q := NewPrioQueue(0, 0, 0, 1, false)
	fillQueue(t, q)
	for i := 0; i < 5; i++ {
		x := q.Pop()
		fmt.Println("fast:", x.IsFastTrack, "high-prio:", x.IsHighPrio)
		require.Equal(t, true, x.IsFastTrack)
		require.Equal(t, true, q.Pop().IsHighPrio)
	}

	// next 9 should all be high-prio
	for i := 0; i < 6; i++ {
		require.Equal(t, true, q.Pop().IsHighPrio)
	}

	// last one should be low-prio
	require.Equal(t, false, q.Pop().IsHighPrio)
	require.Equal(t, 0, len(q.lowPrio))
	require.Equal(t, 0, len(q.highPrio))

	// Test 2 - expected: 2x fastTrack -> 1x highPrio
	q = NewPrioQueue(0, 0, 0, 2, false)
	fillQueue(t, q)
	require.Equal(t, true, q.Pop().IsFastTrack)
	require.Equal(t, true, q.Pop().IsFastTrack)
	require.Equal(t, true, q.Pop().IsHighPrio)
	require.Equal(t, true, q.Pop().IsFastTrack)
	require.Equal(t, true, q.Pop().IsFastTrack)
	require.Equal(t, true, q.Pop().IsHighPrio)
	require.Equal(t, true, q.Pop().IsFastTrack)

	// Test 3 - expected: all fastTrack -> all highPrio
	q = NewPrioQueue(0, 0, 0, 2, true)
	fillQueue(t, q)
	for i := 0; i < 5; i++ {
		require.Equal(t, true, q.Pop().IsFastTrack)
	}
	for i := 0; i < 11; i++ {
		require.Equal(t, true, q.Pop().IsHighPrio)
	}
}

func TestPrioQueueMultipleReaders(t *testing.T) {
	q := NewPrioQueue(0, 0, 0, 2, false)
	taskLowPrio := NewSimRequest(context.Background(), "1", []byte("taskLowPrio"), false, false)

	counts := make(map[int]int)
	resultC := make(chan int, 4)

	// Goroutine that counts the results
	go func() {
		for id := range resultC {
			counts[id]++
		}
	}()

	reader := func(id int) {
		for {
			resp := q.Pop()
			require.NotNil(t, resp)
			resultC <- id
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Start 2 readers
	go reader(1)
	go reader(2)

	// Push 6 tasks
	q.Push(taskLowPrio)
	q.Push(taskLowPrio)
	q.Push(taskLowPrio)
	q.Push(taskLowPrio)
	q.Push(taskLowPrio)
	q.Push(taskLowPrio)

	// Wait a bit for the processing to finish
	time.Sleep(100 * time.Millisecond)

	// Each reader should have processed the same number of tasks
	require.Equal(t, 3, counts[1])
	require.Equal(t, 3, counts[2])
}

func TestPrioQueueVarious(t *testing.T) {
	q := NewPrioQueue(0, 0, 0, 2, false)
	q.Push(nil)
	require.Equal(t, 0, len(q.highPrio))
	require.Equal(t, 0, len(q.lowPrio))

	require.True(t, len(q.String()) > 5)
}

// Test used for benchmark: single reader
func _testPrioQueue1(numWorkers, numItems int) *PrioQueue {
	q := NewPrioQueue(0, 0, 0, 2, false)
	taskLowPrio := NewSimRequest(context.Background(), "1", []byte("taskLowPrio"), false, false)

	var wg sync.WaitGroup

	// Goroutine that drains the queue
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				resp := q.Pop()
				if resp == nil {
					return
				}
			}
		}()
	}

	for i := 0; i < numItems; i++ {
		q.Push(taskLowPrio)
	}

	q.CloseAndWait()
	wg.Wait() // ensure that all workers have finished
	return q
}

func TestPrioQueue1(t *testing.T) {
	q := _testPrioQueue1(1, 1000)
	require.Equal(t, 0, q.NumRequests())

	q = _testPrioQueue1(5, 100)
	require.Equal(t, 0, q.NumRequests())
}

func BenchmarkPrioQueue(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_testPrioQueue1(1, 10_000)
	}
}

func BenchmarkPrioQueueMultiReader(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_testPrioQueue1(5, 10_000)
	}
}
