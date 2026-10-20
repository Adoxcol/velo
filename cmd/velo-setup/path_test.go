package main

import (
	"os"
	"testing"
)

func TestPathAddRemove(t *testing.T) {
	if os.Getenv("VELO_PATH_TEST") == "" {
		t.Skip("changes the user PATH; set VELO_PATH_TEST=1 to run")
	}
	before, _ := readUserPath()
	dir := `C:\velo-path-test`
	if err := setOnPath(dir, true); err != nil {
		t.Fatal(err)
	}
	if !onPath(dir) || !onPath(dir+`\`) {
		t.Fatal("not on PATH after adding")
	}
	setOnPath(dir, true) // no duplicate
	p, _ := readUserPath()
	if n := len(pathParts(p)) - len(pathParts(before)); n != 1 {
		t.Fatalf("added %d entries, want 1", n)
	}
	if err := setOnPath(dir, false); err != nil {
		t.Fatal(err)
	}
	if onPath(dir) {
		t.Fatal("still on PATH after removing")
	}
	if after, _ := readUserPath(); len(pathParts(after)) != len(pathParts(before)) {
		t.Fatal("other PATH entries changed")
	}
}

func TestCompactPathParts(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\Test\AppData\Local`)
	t.Setenv("USERPROFILE", `C:\Users\Test`)
	sys := `C:\Windows\system32;C:\Program Files\Git\cmd`
	user := []string{
		`C:\Windows\system32`,
		`C:\Users\Test\scoop\shims`,
		`C:\Users\Test\scoop\shims\`,
		`C:\Users\Test\AppData\Local\Programs\Velo`,
	}
	got := compactPathParts(user, sys, true)
	if len(got) != 2 || got[0] != `%USERPROFILE%\scoop\shims` || got[1] != `%LOCALAPPDATA%\Programs\Velo` {
		t.Fatalf("compactPathParts = %v", got)
	}
}
