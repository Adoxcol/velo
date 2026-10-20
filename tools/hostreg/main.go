// hostreg connects browsers (Chrome, Edge, Brave, Chromium, Firefox) to the
// velo-host.exe in a folder, for development builds. Run from the repo root:
//
//	go run ./tools/hostreg bin
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Adoxcol/velo/internal/hostreg"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hostreg <folder with velo-host.exe>")
		os.Exit(2)
	}
	dir, err := filepath.Abs(os.Args[1])
	if err == nil {
		err = hostreg.Register(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Browsers now use", filepath.Join(dir, "velo-host.exe"))
}
