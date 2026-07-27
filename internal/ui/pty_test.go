package ui

import (
	"bufio"
	"os/exec"
	"testing"
)

func TestRealPTYSpawnerRunsCommandAndCapturesOutput(t *testing.T) {
	spawner := realPTYSpawner{}
	sess, err := spawner.Start(exec.Command("echo", "hello-pty"))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sess.Close()

	scanner := bufio.NewScanner(sess)
	if !scanner.Scan() {
		t.Fatalf("expected an output line, got none (scanner err: %v)", scanner.Err())
	}
	if got := scanner.Text(); got != "hello-pty" {
		t.Errorf("output line = %q, want %q", got, "hello-pty")
	}
}

func TestRealPTYSpawnerSetsize(t *testing.T) {
	spawner := realPTYSpawner{}
	sess, err := spawner.Start(exec.Command("cat"))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sess.Close()

	if err := sess.Setsize(40, 100); err != nil {
		t.Errorf("Setsize() error = %v, want nil", err)
	}
}
