package ctr

import (
	"context"
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

type nativeCleanupProcess struct {
	containerd.Process
	states    []containerd.ProcessStatus
	killErr   error
	deleteErr error
	killed    bool
	deleted   bool
}

func (p *nativeCleanupProcess) Status(ctx context.Context) (containerd.Status, error) {
	if err := ctx.Err(); err != nil {
		return containerd.Status{}, err
	}
	state := p.states[0]
	if len(p.states) > 1 {
		p.states = p.states[1:]
	}
	return containerd.Status{Status: state}, nil
}

func (p *nativeCleanupProcess) Kill(_ context.Context, signal syscall.Signal, _ ...containerd.KillOpts) error {
	if signal != syscall.SIGKILL {
		return errors.New("unexpected signal")
	}
	p.killed = true
	return p.killErr
}

func (p *nativeCleanupProcess) Delete(_ context.Context, _ ...containerd.ProcessDeleteOpts) (*containerd.ExitStatus, error) {
	p.deleted = true
	return nil, p.deleteErr
}

func TestNativeCleanupKillsRunningProcessAndUsesLiveContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	process := &nativeCleanupProcess{states: []containerd.ProcessStatus{containerd.Running, containerd.Stopped}}
	if err := cleanupNativeProcess(ctx, process); err != nil {
		t.Fatal(err)
	}
	if !process.killed || !process.deleted {
		t.Fatalf("cleanup: killed=%t deleted=%t", process.killed, process.deleted)
	}
}

func TestNativeCleanupDoesNotDeleteAfterKillFailure(t *testing.T) {
	want := errors.New("synthetic kill failure")
	process := &nativeCleanupProcess{states: []containerd.ProcessStatus{containerd.Running}, killErr: want}
	if err := cleanupNativeProcess(context.Background(), process); !errors.Is(err, want) {
		t.Fatalf("cleanup error = %v", err)
	}
	if process.deleted {
		t.Fatal("deleted process after failed kill")
	}
}

type nativeWriterFunc func([]byte) (int, error)

func (f nativeWriterFunc) Write(p []byte) (int, error) { return f(p) }

type completedNativeIO struct{ cio.IO }

func (completedNativeIO) Wait() {}

func TestNativeExecReportsOutputFailures(t *testing.T) {
	failure := errors.New("synthetic output failure")
	for _, tc := range []struct {
		name   string
		writer nativeWriterFunc
		want   error
	}{
		{name: "write error", writer: func([]byte) (int, error) { return 0, failure }, want: failure},
		{name: "short write", writer: func([]byte) (int, error) { return 1, nil }, want: io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout := trackNativeOutput(tc.writer)
			_, _ = stdout.Write([]byte("output"))
			status := make(chan containerd.ExitStatus, 1)
			status <- *containerd.NewExitStatus(0, time.Time{}, nil)
			_, err := waitNativeExec(context.Background(), status, completedNativeIO{}, stdout, trackNativeOutput(nil))
			if !errors.Is(err, tc.want) {
				t.Fatalf("output error = %v", err)
			}
		})
	}
}

func TestNativeExecReturnsExitStatus(t *testing.T) {
	status := make(chan containerd.ExitStatus, 1)
	status <- *containerd.NewExitStatus(7, time.Time{}, nil)
	code, err := waitNativeExec(context.Background(), status, completedNativeIO{}, trackNativeOutput(nil), trackNativeOutput(nil))
	if err != nil || code != 7 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	close(status)
	if _, err := waitNativeExec(context.Background(), status, completedNativeIO{}, trackNativeOutput(nil), trackNativeOutput(nil)); err == nil {
		t.Fatal("closed status channel reported success")
	}
}

type waitingNativeIO struct {
	cio.IO
	started chan struct{}
	stopped chan struct{}
}

func (o *waitingNativeIO) Wait() {
	close(o.started)
	<-o.stopped
}

func (o *waitingNativeIO) Cancel() { close(o.stopped) }

func TestNativeExecCancelsOutputDrainAfterProcessExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status := make(chan containerd.ExitStatus, 1)
	status <- *containerd.NewExitStatus(0, time.Time{}, nil)
	output := &waitingNativeIO{started: make(chan struct{}), stopped: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := waitNativeExec(ctx, status, output, trackNativeOutput(nil), trackNativeOutput(nil))
		done <- err
	}()
	select {
	case <-output.started:
	case <-time.After(5 * time.Second):
		t.Fatal("output drain did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("output drain did not stop after cancellation")
	}
}

func TestCancelableOutputRejectsUninterruptibleWriter(t *testing.T) {
	writer := nativeWriterFunc(func(p []byte) (int, error) { return len(p), nil })
	if _, err := cancelableOutput(context.Background(), writer); err == nil {
		t.Fatal("accepted writer without cancellation support")
	}
	if output, err := cancelableOutput(context.Background(), nil); err != nil || output != io.Discard {
		t.Fatalf("nil output: %v, %v", output, err)
	}
}

type cancelDuringWrite struct {
	cancel context.CancelFunc
	calls  int
}

func (w *cancelDuringWrite) Write([]byte) (int, error) {
	return 0, errors.New("unexpected plain write")
}
func (w *cancelDuringWrite) WriteContext(ctx context.Context, _ []byte) (int, error) {
	w.calls++
	w.cancel()
	return 0, ctx.Err()
}

func TestCancelableOutputDrainsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	destination := &cancelDuringWrite{cancel: cancel}
	output, err := cancelableOutput(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if n, err := output.Write([]byte("discard")); err != nil || n != 7 {
			t.Fatalf("write %d: n=%d err=%v", i, n, err)
		}
	}
	if destination.calls != 1 {
		t.Fatalf("canceled destination called %d times", destination.calls)
	}
}

func TestMergeExecEnvReplacesInPlaceAndAppends(t *testing.T) {
	got := mergeExecEnv([]string{"A=1", "B=2", "PATH=/bin"}, []string{"B=20", "C=3", "A=10"})
	want := []string{"A=10", "B=20", "PATH=/bin", "C=3"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("env=%q want %q", got, want)
	}
}

func TestValidateExecEnvRejectsWithoutLeakingValues(t *testing.T) {
	for _, entry := range []string{"HOST_ONLY", "1BAD=value", "A=value\x00secret", "A=value"} {
		err := validateExecEnv([]string{entry})
		if entry == "A=value" {
			if err != nil {
				t.Fatalf("valid entry rejected: %v", err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("entry %q was accepted", entry)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("error leaked a value: %v", err)
		}
	}
}

func TestParseExecUserAndApply(t *testing.T) {
	for _, tc := range []struct {
		user   string
		uid    uint32
		gid    uint32
		hasGID bool
		ok     bool
	}{
		{"", 0, 0, false, true},
		{"0", 0, 0, false, true},
		{"1000:1001", 1000, 1001, true, true},
		{"4294967295:4294967295", 4294967295, 4294967295, true, true},
		{"root", 0, 0, false, false},
		{"1000:x", 0, 0, false, false},
		{"1000:", 0, 0, false, false},
		{":1000", 0, 0, false, false},
		{"1:2:3", 0, 0, false, false},
	} {
		uid, gid, hasGID, err := parseExecUser(tc.user)
		if (err == nil) != tc.ok || uid != tc.uid || gid != tc.gid || hasGID != tc.hasGID {
			t.Fatalf("parseExecUser(%q) = %d,%d,%t,%v", tc.user, uid, gid, hasGID, err)
		}
	}
	process := &specs.Process{User: specs.User{UID: 1, GID: 2}}
	if err := applyExecUser(process, "1000"); err != nil || process.User.UID != 1000 || process.User.GID != 2 {
		t.Fatalf("uid-only override: %#v err=%v", process.User, err)
	}
	if err := applyExecUser(process, "1000:1001"); err != nil || process.User.UID != 1000 || process.User.GID != 1001 {
		t.Fatalf("uid/gid override: %#v err=%v", process.User, err)
	}
}

type stuckNativeIO struct {
	cio.IO
	cancelCalled chan struct{}
	release      chan struct{}
}

func (o *stuckNativeIO) Wait() { <-o.release }

func (o *stuckNativeIO) Cancel() {
	close(o.cancelCalled)
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(o.release)
	}()
}

func TestNativeExecDetachesOutputHoldersAfterDrainGrace(t *testing.T) {
	oldGrace := nativeExecDrainGrace
	nativeExecDrainGrace = 50 * time.Millisecond
	defer func() { nativeExecDrainGrace = oldGrace }()
	status := make(chan containerd.ExitStatus, 1)
	status <- *containerd.NewExitStatus(3, time.Time{}, nil)
	output := &stuckNativeIO{cancelCalled: make(chan struct{}), release: make(chan struct{})}
	code, err := waitNativeExec(context.Background(), status, output, trackNativeOutput(nil), trackNativeOutput(nil))
	if err != nil {
		t.Fatalf("grace expiry must not fail exec: %v", err)
	}
	if code != 3 {
		t.Fatalf("code=%d", code)
	}
	select {
	case <-output.cancelCalled:
	default:
		t.Fatal("FIFOs not canceled after grace expiry")
	}
}
