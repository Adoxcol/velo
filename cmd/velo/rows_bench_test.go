package main

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Adoxcol/velo/internal/engine"
)

func BenchmarkRows(b *testing.B) {
	for _, n := range []int{10, 100, 500, 1000} {
		b.Run(fmt.Sprintf("items-%d", n), func(b *testing.B) {
			a := &app{}
			for i := 1; i <= n; i++ {
				d := &engine.Download{
					ID:    i,
					Req:   engine.Request{URL: "https://example.com/downloads/file" + strconv.Itoa(i) + ".zip"},
					Path:  "/downloads/file" + strconv.Itoa(i) + ".zip",
					Total: 1024 * 1024,
				}
				if i%2 == 0 {
					d.Status = "downloading"
				} else {
					d.Status = "queued"
				}
				it := &item{
					d:        d,
					added:    time.Now(),
					lastDone: 512 * 1024,
					speed:    102400,
				}
				if i%2 == 0 {
					it.running = true
				}
				a.items = append(a.items, it)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = a.rows()
			}
		})
	}
}

func BenchmarkRowsUncachedURLParse(b *testing.B) {
	for _, n := range []int{10, 100, 500, 1000} {
		b.Run(fmt.Sprintf("items-%d", n), func(b *testing.B) {
			a := &app{}
			for i := 1; i <= n; i++ {
				d := &engine.Download{
					ID:     i,
					Req:    engine.Request{URL: "https://example.com/downloads/file" + strconv.Itoa(i) + ".zip?token=123#section"},
					Status: "queued",
				}
				it := &item{
					d:     d,
					added: time.Now(),
				}
				a.items = append(a.items, it)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				// Simulate cache invalidation/re-evaluation by clearing cachedName or cachedURL
				for _, it := range a.items {
					it.cachedName = ""
				}
				_ = a.rows()
			}
		})
	}
}
