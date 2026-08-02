// SPDX-License-Identifier: MIT
package dream

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingInvoker records the peak number of concurrent Invoke calls.
type countingInvoker struct {
	live int32
	peak int32
	hold time.Duration
}

func (ci *countingInvoker) Invoke(prompt string, tier string) (InvokeResult, error) {
	n := atomic.AddInt32(&ci.live, 1)
	for {
		peak := atomic.LoadInt32(&ci.peak)
		if n <= peak || atomic.CompareAndSwapInt32(&ci.peak, peak, n) {
			break
		}
	}
	time.Sleep(ci.hold)
	atomic.AddInt32(&ci.live, -1)
	return InvokeResult{Response: "{}"}, nil
}

func (ci *countingInvoker) Model() string { return "counting" }

func TestGated_CapsConcurrentInvocations(t *testing.T) {
	inner := &countingInvoker{hold: 5 * time.Millisecond}
	inv := Gated(inner, 2)

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := inv.Invoke("p", "sonnet"); err != nil {
				t.Errorf("Invoke: %v", err)
			}
		}()
	}
	wg.Wait()

	if inner.peak > 2 {
		t.Errorf("peak concurrency: got %d, want <= 2", inner.peak)
	}
}

func TestGated_ZeroMeansUnlimited(t *testing.T) {
	inner := &countingInvoker{}
	if got := Gated(inner, 0); got != Invoker(inner) {
		t.Errorf("Gated(inv, 0) should return the invoker unwrapped, got %T", got)
	}
}

func TestGated_PreservesModel(t *testing.T) {
	if got := Gated(&countingInvoker{}, 1).Model(); got != "counting" {
		t.Errorf("Model: got %q", got)
	}
}
