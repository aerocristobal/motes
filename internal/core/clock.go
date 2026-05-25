// SPDX-License-Identifier: MIT
package core

import "time"

// Clock abstracts time.Now so callers (most importantly MoteManager) can be
// driven with a deterministic clock under test. Production code uses
// RealClock; tests use a fake that returns a configurable instant.
type Clock interface {
	Now() time.Time
}

// RealClock returns the host wall clock.
type RealClock struct{}

// Now returns time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// FixedClock is a test helper that returns a frozen time. Exported so tests
// in other packages (cmd/mote) can construct one without duplicating the type.
type FixedClock struct {
	T time.Time
}

// Now returns the frozen time.
func (c FixedClock) Now() time.Time { return c.T }
