package main

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Adoxcol/velo/internal/engine"
)

func BenchmarkSaveHistoryUnchanged(b *testing.B) {
	for _, n := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("items-%d", n), func(b *testing.B) {
			home := b.TempDir()
			a := &app{
				home: home,
			}
			for i := 1; i <= n; i++ {
				d := &engine.Download{
					ID:    i,
					Req:   engine.Request{URL: "https://example.com/file" + strconv.Itoa(i) + ".zip"},
					Path:  "/downloads/file" + strconv.Itoa(i) + ".zip",
					Total: 1024 * 1024,
				}
				d.SetStatus("done")
				it := &item{
					d:        d,
					added:    time.Now(),
					finished: time.Now(),
				}
				a.items = append(a.items, it)
			}

			// First call populates history
			a.saveHistory()

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				a.saveHistory()
			}
		})
	}
}
