// velo is the download manager app for Windows: a tray
// icon plus a download window. It accepts jobs from velo-host over an
// authenticated localhost endpoint and runs them with the segmented engine.
// Starting it again while it runs just opens the window.
// The command-line companion is veloctl.
package main

// Embed the icon in velo.exe (Explorer, Start menu). Regenerate after
// changing the icon: go run ./tools/mkicon && go generate ./cmd/velo
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico assets/velo.ico -o rsrc_windows_amd64.syso

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"github.com/Adoxcol/velo/internal/engine"
	"github.com/Adoxcol/velo/internal/hostreg"
	"github.com/Adoxcol/velo/internal/ipc"
)

func main() {
	home, _ := os.UserHomeDir()
	dir := flag.String("dir", filepath.Join(home, "Downloads", "Velo"), "download folder")
	segs := flag.Int("segments", 16, "connections per download")
	background := flag.Bool("background", false, "start in the tray without opening the window")
	flag.Parse()

	if !singleInstance() {
		// Already running: ask it to open its window (it may still be starting).
		for i := 0; i < 20; i++ {
			if _, err := ipc.Call("show", "", nil); err == nil {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		return
	}

	portableDir, portable := ipc.Portable()
	if !portable {
		migrateFromMyDM()
	}
	os.MkdirAll(ipc.Dir(), 0o700)
	if f, err := os.OpenFile(filepath.Join(ipc.Dir(), "velo.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(f)
	}
	// A portable copy connects the browser extension to itself when run.
	if portable && !hostreg.RegisteredTo(portableDir) {
		if err := hostreg.Register(portableDir); err != nil {
			log.Printf("portable: couldn't connect browsers: %v", err)
		} else {
			log.Printf("portable: browsers now use %s", portableDir)
		}
	}

	eng := engine.New(*dir)
	eng.SetSegments(*segs)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	a := newApp(eng)
	ep := ipc.Endpoint{Port: ln.Addr().(*net.TCPAddr).Port, Token: a.token, PID: os.Getpid()}
	if err := ipc.WriteEndpoint(ep); err != nil {
		log.Fatal(err)
	}
	log.Printf("Velo listening on 127.0.0.1:%d, saving to %s", ep.Port, eng.Dir())
	srv := &http.Server{Handler: http.HandlerFunc(a.handle), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second}
	go srv.Serve(ln)

	runTray(a, !*background) // blocks until Exit
	ipc.RemoveEndpoint()
}

// singleInstance takes a per-user named mutex; false if another Velo holds it.
// The handle is kept open for the life of the process.
func singleInstance() bool {
	name, _ := syscall.UTF16PtrFromString(`Local\Velo.SingleInstance`)
	h, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW").
		Call(0, 0, uintptr(unsafe.Pointer(name)))
	return h != 0 && err != syscall.Errno(183) // ERROR_ALREADY_EXISTS
}
