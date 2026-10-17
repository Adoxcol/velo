// veloctl is the command-line companion to the Velo tray app.
//
//	veloctl list               show downloads
//	veloctl add <url>          add a download
//	veloctl pause <id>         pause a download
//	veloctl resume <id>        resume a paused/failed download
//	veloctl remove <id>        remove from the list (deletes unfinished partial files)
//	veloctl limit <KB/s>       set the speed limit for all downloads (0 = none)
//	veloctl get <url> [dir]    download in this console without the app (Ctrl+C pauses)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"velo/internal/engine"
	"velo/internal/ipc"
	"velo/internal/version"
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
	fmt.Println("usage: veloctl list | add <url> | pause <id> | resume <id> | remove <id> | limit <KB/s> | get <url> [dir]")
	os.Exit(2)
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
		fmt.Println("Velo " + version.Version)
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
		if arg == "" {
			usage()
		}
		job := ipc.Job{Type: "download", URL: arg}
		job.Kind = streamKind(arg)
		body, _ := json.Marshal(job)
		reply(ipc.Call("add", "", body))
	case "pause", "resume", "remove", "limit":
		if arg == "" {
			usage()
		}
		reply(ipc.Call(cmd, arg, nil))
	case "get":
		if arg == "" {
			usage()
		}
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, "Downloads", "Velo")
		if len(os.Args) > 3 {
			dir = os.Args[3]
		}
		get(engine.New(dir), arg)
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

// get downloads url in the console, resuming a matching paused download.
func get(eng *engine.Engine, url string) {
	d := &engine.Download{Req: engine.Request{URL: url}}
	d.Req.Kind = streamKind(url)
	for _, p := range engine.FindPaused(eng.Dir()) {
		if p.Req.URL == url {
			d = p
			break
		}
	}
	pct := d.Progress
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Printf("\r%5.1f%%  %d connections ", pct(), d.Connections())
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
	fmt.Printf("%s  %d bytes  %.1fs\n", p, d.Done.Load(), time.Since(t).Seconds())
}

// streamKind guesses the download kind from the URL.
func streamKind(u string) string {
	_, kind := engine.Downloadable(u)
	return kind
}
