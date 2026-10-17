//go:build linux || darwin

package engine

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestUnixDiskSpace(t *testing.T) {
	if freeSpace(t.TempDir()) <= 0 {
		t.Fatal("could not read free disk space")
	}
	if freeSpace("/velo-nonexistent-directory/test") != -1 {
		t.Fatal("expected unknown space for nonexistent path")
	}
}

func TestUnixCancelProcessGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 30 & wait")
	hideWindow(cmd)
	cmd.Cancel = func() error { return killProcessTree(cmd) }
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled command succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process group did not terminate")
	}
}
