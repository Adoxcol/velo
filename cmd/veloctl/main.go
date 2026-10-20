// veloctl is the command-line companion to the Velo tray app.
//
//	veloctl list                               show downloads
//	veloctl add [-format FMT] [-i FILE] [url]  add a download to Velo
//	veloctl pause <id>                         pause a download
//	veloctl resume <id>                        resume a paused/failed download
//	veloctl remove <id>                        remove from the list (deletes unfinished partial files)
//	veloctl limit <KB/s>                       set the speed limit for all downloads (0 = none)
//	veloctl get [-format FMT] [-i FILE] [url] [dir]  download in this console without the app (Ctrl+C pauses)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adoxcol/velo/internal/engine"
	"github.com/Adoxcol/velo/internal/ipc"
	"github.com/Adoxcol/velo/internal/version"
)

type row struct {
	ID          int     `json:"id"`
	Status      string  `json:"status"`
	Percent     float64 `json:"percent"`
	Speed       float64 `json:"speed"`
	Connections int     `json:"connections"`
	File        string  `json:"file"`
	Error       string  `json:"error"`
}

func usage() {
	fmt.Println("usage: veloctl list | add [-format FMT] [-i FILE] [url] | pause <id> | resume <id> | remove <id> | limit <KB/s> | get [-format FMT] [-i FILE] [url] [dir]")
	os.Exit(2)
}

func parseFlags(args []string) (urls []string, dir, format string) {
	fs := flag.NewFlagSet("veloctl", flag.ExitOnError)
	fs.Usage = usage
	var inFile string
	fs.StringVar(&format, "format", "", "video quality/format: 1080p, 720p, 480p, or audio")
	fs.StringVar(&format, "f", "", "shorthand for -format")
	fs.StringVar(&inFile, "i", "", "file containing URLs to download (one per line)")
	_ = fs.Parse(args)
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "", "1080p", "720p", "480p", "audio":
	case "mp3":
		format = "audio"
	default:
		fmt.Printf("unknown format %q (expected 1080p, 720p, 480p, or audio)\n", format)
		os.Exit(2)
	}
	rest := fs.Args()
	if inFile != "" {
		var err error
		urls, err = readURLFile(inFile)
		check(err)
		if len(rest) > 0 {
			dir = rest[0]
		}
	} else if len(rest) > 0 {
		urls = []string{rest[0]}
		if len(rest) > 1 {
			dir = rest[1]
		}
	}
	if len(urls) == 0 {
		usage()
	}
	return urls, dir, format
}

func readURLFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var urls []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			urls = append(urls, line)
		}
	}
	return urls, sc.Err()
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, arg := os.Args[1], ""
	if len(os.Args) > 2 {
		arg = os.Args[2]
	}
	switch cmd {
	case "version", "--version", "-version":
		fmt.Println("Velo " + version.Display())
	case "list":
		b, err := ipc.Call("list", "", nil)
		check(err)
		var rows []row
		json.Unmarshal(b, &rows)
		if len(rows) == 0 {
			fmt.Println("no downloads")
		}
		for _, r := range rows {
			speed := ""
			if r.Speed > 0 {
				speed = fmt.Sprintf("%.1f MB/s", r.Speed/1e6)
			}
			fmt.Printf("#%-3d %-11s %5.1f%%  %-10s %d conn  %s  %s\n", r.ID, r.Status, r.Percent, speed, r.Connections, r.File, r.Error)
		}
	case "add":
		urls, _, format := parseFlags(os.Args[2:])
		for _, u := range urls {
			job := ipc.Job{Type: "download", URL: u, Kind: streamKind(u), Format: format}
			body, _ := json.Marshal(job)
			reply(ipc.Call("add", "", body))
		}
	case "pause", "resume", "remove", "limit":
		if arg == "" {
			usage()
		}
		reply(ipc.Call(cmd, arg, nil))
	case "get":
		urls, dir, format := parseFlags(os.Args[2:])
		if dir == "" {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, "Downloads", "Velo")
		}
		eng := engine.New(dir)
		for _, u := range urls {
			get(eng, u, format)
		}
	default:
		usage()
	}
}

func check(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func reply(b []byte, err error) {
	check(err)
	var r ipc.Reply
	json.Unmarshal(b, &r)
	if !r.OK {
		fmt.Println("error:", r.Error)
		os.Exit(1)
	}
	fmt.Println("ok", r.ID)
}

func fmtBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// get downloads url in the console, resuming a matching paused download.
func get(eng *engine.Engine, url, format string) {
	d := &engine.Download{Req: engine.Request{URL: url, Kind: streamKind(url), Format: format}}
	for _, p := range engine.FindPaused(eng.Dir()) {
		if p.Req.URL == url && p.Req.Format == format {
			d = p
			break
		}
	}
	pct := d.Progress
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lastBytes := d.Done.Load()
		lastAt := time.Now()
		var smoothSpeed float64
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				cur := d.Done.Load()
				if dt := now.Sub(lastAt).Seconds(); dt > 0 {
					inst := float64(max(cur-lastBytes, 0)) / dt
					if smoothSpeed == 0 {
						smoothSpeed = inst
					} else {
						smoothSpeed = 0.35*inst + 0.65*smoothSpeed
					}
					lastBytes, lastAt = cur, now
				}
				p := pct()
				filled := min(max(int(p/100*16+0.5), 0), 16)
				bar := strings.Repeat("█", filled) + strings.Repeat("░", 16-filled)
				_, total := d.Info()
				sizeStr := fmtBytes(cur)
				if total > 0 {
					sizeStr += "/" + fmtBytes(total)
				}
				spdStr := ""
				etaStr := ""
				if smoothSpeed > 0 {
					spdStr = fmt.Sprintf("  %.1f MB/s", smoothSpeed/1e6)
					if total > cur {
						rem := int(float64(total-cur)/smoothSpeed + 0.5)
						etaStr = fmt.Sprintf("  ETA %ds", rem)
					}
				}
				fmt.Printf("\r[%s] %5.1f%%  %s%s%s  (%d conn)   ", bar, p, sizeStr, spdStr, etaStr, d.Connections())
			}
		}
	}()
	t := time.Now()
	err := eng.Run(ctx, d)
	fmt.Println()
	if errors.Is(err, context.Canceled) {
		fmt.Printf("paused at %.1f%%; run the same command to resume\n", pct())
		return
	}
	check(err)
	p, _ := d.Info()
	fmt.Printf("%s  %s  %.1fs\n", p, fmtBytes(d.Done.Load()), time.Since(t).Seconds())
}

// streamKind guesses the download kind from the URL.
func streamKind(u string) string {
	_, kind := engine.Downloadable(u)
	return kind
}
