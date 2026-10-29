package ctr

import (
	"context"
	"errors"
	"strings"
	"testing"

	runtime "github.com/KLORQ/sandboxes-runtime"
)

type fakeRunner struct {
	calls [][]string
	fn    func(args []string) ([]byte, error)
}

func (f *fakeRunner) CombinedOutput(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	if f.fn == nil {
		return nil, nil
	}
	return f.fn(call)
}

func newTestClient(t *testing.T, runner Runner) *Client {
	t.Helper()
	client, err := New(Config{Socket: "/tmp/containerd.sock", CTR: "ctr"})
	if err != nil {
		t.Fatal(err)
	}
	client.runner = runner
	return client
}

func TestRunBuildsCommand(t *testing.T) {
	runner := &fakeRunner{}
	client := newTestClient(t, runner)
	err := client.Run(context.Background(), RunSpec{
		ContainerID: "klorq-dev",
		Name:        "dev",
		Image:       "alpine:3.22",
		CPUs:        2,
		MemoryMiB:   512,
		FIFODir:     "/tmp/fifo",
		Mounts:      []Mount{{Source: "/work", Destination: "/work"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d", len(runner.calls))
	}
	args := strings.Join(runner.calls[0], " ")
	for _, want := range []string{
		"ctr --address /tmp/containerd.sock --namespace klorq run --detach",
		"--runtime io.containerd.nerdbox.v1",
		"--snapshotter erofs",
		"--fifo-dir /tmp/fifo",
		"type=bind,src=/work,dst=/work,options=rbind:rw",
		"alpine:3.22 klorq-dev /bin/sh -c",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %q in %q", want, args)
		}
	}
}

func TestEnsureImage(t *testing.T) {
	runner := &fakeRunner{fn: func(args []string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "images ls") {
			return []byte("alpine:3.22\n"), nil
		}
		return nil, nil
	}}
	client := newTestClient(t, runner)
	if err := client.EnsureImage(context.Background(), "alpine:3.22"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("unexpected pull: %d calls", len(runner.calls))
	}

	runner.calls = nil
	runner.fn = func(args []string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "images ls") {
			return nil, nil
		}
		if strings.Contains(strings.Join(args, " "), "images pull") {
			return nil, nil
		}
		return nil, errors.New("unexpected")
	}
	if err := client.EnsureImage(context.Background(), "alpine:3.22"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %d", len(runner.calls))
	}
}

func TestBackendCreate(t *testing.T) {
	runner := &fakeRunner{fn: func(args []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "images ls") || strings.Contains(joined, "containers ls") {
			return nil, nil
		}
		return nil, nil
	}}
	client := newTestClient(t, runner)
	backend := &Backend{client: client, image: "alpine:3.22", fifoDir: "/tmp/fifo"}
	handle, err := backend.Create(context.Background(), runtime.Config{
		Name:      "dev",
		Workspace: "/work",
		CPUs:      2,
		MemoryMiB: 512,
	})
	if err != nil {
		t.Fatal(err)
	}
	if handle.ID != "klorq-dev" || handle.FIFODir != "/tmp/fifo" {
		t.Fatalf("unexpected handle: %+v", handle)
	}
}

func TestCapabilitiesFailClosed(t *testing.T) {
	backend := &Backend{}
	caps, err := backend.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Network || caps.IPv6 {
		t.Fatalf("network capabilities must stay off: %+v", caps)
	}
}

func TestBackendHandle(t *testing.T) {
	backend := &Backend{fifoDir: "/tmp/fifo"}
	handle := backend.Handle("dev")
	if handle.ID != "klorq-dev" || handle.FIFODir != "/tmp/fifo" {
		t.Fatalf("unexpected handle: %+v", handle)
	}
}
