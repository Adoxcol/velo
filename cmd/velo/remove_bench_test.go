package main

import (
	"context"
	"testing"
	"time"

	"github.com/Adoxcol/velo/internal/engine"
)

func BenchmarkRemoveRunning(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		a := &app{byID: make(map[int]*item), wake: make(chan struct{}, 1)}
		d := &engine.Download{ID: 1}
		ctx, cancel := context.WithCancel(context.Background())
		it := &item{
			d:       d,
			cancel:  cancel,
			running: true,
		}
		a.items = append(a.items, it)
		a.byID[1] = it

		// Goroutine simulating engine stopping 1ms after context cancellation
		go func() {
			<-ctx.Done()
			time.Sleep(1 * time.Millisecond)
			a.mu.Lock()
			it.running = false
			a.cond().Broadcast()
			a.mu.Unlock()
		}()

		err := a.remove(1)
		if err != nil {
			b.Fatalf("remove failed: %v", err)
		}
	}
}
