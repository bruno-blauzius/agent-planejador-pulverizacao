// Package usecase holds the application rules of the API. Interfaces needed by
// use cases are declared here, in the consumer package.
package usecase

import "sync/atomic"

// Lifecycle tracks the process phases that Kubernetes probes observe.
// It is created once in the composition root and injected where needed.
type Lifecycle struct {
	started      atomic.Bool
	shuttingDown atomic.Bool
}

// NewLifecycle returns a Lifecycle in the "starting" phase.
func NewLifecycle() *Lifecycle {
	return &Lifecycle{}
}

// MarkStarted signals that initialization finished and the listener is bound.
func (l *Lifecycle) MarkStarted() { l.started.Store(true) }

// BeginShutdown signals draining: readiness must fail so traffic stops arriving.
func (l *Lifecycle) BeginShutdown() { l.shuttingDown.Store(true) }

// IsStarted reports whether initialization has finished.
func (l *Lifecycle) IsStarted() bool { return l.started.Load() }

// IsShuttingDown reports whether the process is draining.
func (l *Lifecycle) IsShuttingDown() bool { return l.shuttingDown.Load() }
