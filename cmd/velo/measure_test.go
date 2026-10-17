package main

import (
	"testing"
	"velo/internal/engine"
)

func BenchmarkMeasure(b *testing.B) {
	app := &app{
		items: make([]*item, 1000),
	}
	for i := 0; i < 1000; i++ {
		d := &engine.Download{}
		d.ID = i + 1
		it := &item{
			d:       d,
			running: i == 0, // 1 running, 999 non-running
		}
		app.items[i] = it
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.stepMeasure()
	}
}
