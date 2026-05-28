package maplibre

import (
	"errors"
	stdruntime "runtime"
	"testing"
	"time"
)

func TestRuntimeCreateWithOptions(t *testing.T) {
	runtime, err := NewRuntimeWithOptions(RuntimeOptions{CachePath: ":memory:"}.WithMaximumCacheSize(0))
	if err != nil {
		t.Fatalf("NewRuntimeWithOptions(): %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
}
func TestRuntimeOptionsRejectEmbeddedNUL(t *testing.T) {
	_, err := NewRuntimeWithOptions(RuntimeOptions{AssetPath: "asset\x00root"})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewRuntimeWithOptions embedded NUL error = %v, want ErrInvalidArgument", err)
	}
}
func TestRuntimeAmbientCacheOperationDiscard(t *testing.T) {
	runtime, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime(): %v", err)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("Close(): %v", err)
		}
	}()

	operation, err := runtime.StartAmbientCacheOperation(AmbientCacheOperationClear)
	if err != nil {
		t.Fatalf("StartAmbientCacheOperation(): %v", err)
	}
	if operation.ID() == 0 {
		t.Fatal("operation ID is zero")
	}
	if err := operation.Discard(); err != nil {
		t.Fatalf("Discard(): %v", err)
	}
	if err := operation.Discard(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("second Discard() error = %v, want ErrInvalidArgument", err)
	}
}
func TestRuntimeAmbientCacheOperationRejectsUnknownOperation(t *testing.T) {
	runtime, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime(): %v", err)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("Close(): %v", err)
		}
	}()

	_, err = runtime.StartAmbientCacheOperation(AmbientCacheOperation(999_999))
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("StartAmbientCacheOperation(unknown) error = %v, want ErrInvalidArgument", err)
	}
}
func TestRuntimeRunBlocking(t *testing.T) {
	// libuv loops are not safe to access concurrently — uv_run with UV_RUN_ONCE
	// must execute on the thread that owns the loop. Pin this goroutine so the
	// loop's NewRuntime/RunBlocking/Close all land on the same OS thread.
	stdruntime.LockOSThread()
	defer stdruntime.UnlockOSThread()

	runtime, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime(): %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	// timeout=0 degrades to the non-blocking RunOnce path.
	hadEvent, err := runtime.RunBlocking(0)
	if err != nil {
		t.Fatalf("RunBlocking(0): %v", err)
	}
	if !hadEvent {
		t.Fatal("RunBlocking(0) reported hadEvent=false; expected true to match runOnce() semantics")
	}

	// With a small positive budget on an idle runtime, the call should block
	// until the deadline expires and report hadEvent=false. We allow the
	// elapsed time some slack on either side: the kernel timer granularity
	// can let us return slightly early on some hosts.
	const budget = 25 * time.Millisecond
	start := time.Now()
	hadEvent, err = runtime.RunBlocking(budget)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunBlocking(%s): %v", budget, err)
	}
	if hadEvent {
		t.Fatalf("RunBlocking(%s) on idle runtime reported hadEvent=true", budget)
	}
	if elapsed < budget/2 {
		t.Fatalf("RunBlocking(%s) returned after only %s; expected to block close to the budget", budget, elapsed)
	}
	if elapsed > budget*4 {
		t.Fatalf("RunBlocking(%s) returned after %s; far longer than the budget", budget, elapsed)
	}
}

func TestRuntimeRunBlockingNilHandle(t *testing.T) {
	var nilRuntime *RuntimeHandle
	if _, err := nilRuntime.RunBlocking(10 * time.Millisecond); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil RuntimeHandle.RunBlocking() error = %v, want ErrInvalidArgument", err)
	}
}

func TestRuntimeCreateRunOnceAndClose(t *testing.T) {
	runtime, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime(): %v", err)
	}
	if err := runtime.RunOnce(); err != nil {
		t.Fatalf("RunOnce(): %v", err)
	}
	if event, err := runtime.PollEvent(); err != nil {
		t.Fatalf("PollEvent(): %v", err)
	} else if event != nil && event.PayloadSize > 0 && event.PayloadType == RuntimeEventPayloadNone {
		t.Fatalf("PollEvent() payload metadata inconsistent: %#v", event)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second Close(): %v", err)
	}
	if _, err := runtime.NewMap(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewMap() after Close error = %v, want ErrInvalidArgument", err)
	}
}
func TestRuntimeCloseWrongThreadLeavesHandleRetryable(t *testing.T) {
	stdruntime.LockOSThread()
	defer stdruntime.UnlockOSThread()

	runtime, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime(): %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- runtime.Close()
	}()
	if err := <-errCh; !errors.Is(err, ErrWrongThread) {
		_ = runtime.Close()
		t.Fatalf("Close() from another thread error = %v, want ErrWrongThread", err)
	}
	if err := runtime.RunOnce(); err != nil {
		_ = runtime.Close()
		t.Fatalf("RunOnce() after failed close: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() on owner thread after failed close: %v", err)
	}
}
