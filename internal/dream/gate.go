// SPDX-License-Identifier: MIT
package dream

// gatedInvoker caps how many invocations may be in flight at once. The batch
// semaphore in the orchestrator only bounds batches; lens mode fans out one
// goroutine per lens inside each batch slot, so real concurrency is
// batching.max_concurrent × len(lenses). Self-hosted backends (llama-swap,
// vLLM, Ollama) answer that burst with 429/502 rather than queueing it, so the
// cap belongs at the request level where every caller funnels through.
//
// A slot is held across retries so a backend that is still warming up sees a
// steady trickle instead of a fresh burst on every attempt.
type gatedInvoker struct {
	inner Invoker
	slots chan struct{}
}

var _ Invoker = (*gatedInvoker)(nil)

// Gated wraps inv so at most maxInFlight invocations run concurrently.
// maxInFlight <= 0 means unlimited, in which case inv is returned unwrapped.
func Gated(inv Invoker, maxInFlight int) Invoker {
	if maxInFlight <= 0 {
		return inv
	}
	return &gatedInvoker{
		inner: inv,
		slots: make(chan struct{}, maxInFlight),
	}
}

func (gi *gatedInvoker) Invoke(prompt string, tier string) (InvokeResult, error) {
	gi.slots <- struct{}{}
	defer func() { <-gi.slots }()
	return gi.inner.Invoke(prompt, tier)
}

func (gi *gatedInvoker) Model() string { return gi.inner.Model() }
