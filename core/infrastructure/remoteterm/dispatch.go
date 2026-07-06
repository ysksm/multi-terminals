package remoteterm

import (
	"context"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// Compile-time interface assertion.
var _ port.TerminalRunner = (*DispatchRunner)(nil)

// DispatchRunner routes TerminalStartRequests by RemoteHost: empty host goes
// to the local runner (PTY on this machine), non-empty host goes to the
// remote runner (terminal on another multi-terminals instance).
type DispatchRunner struct {
	local  port.TerminalRunner
	remote port.TerminalRunner
}

// NewDispatchRunner returns a DispatchRunner over the two runners.
func NewDispatchRunner(local, remote port.TerminalRunner) *DispatchRunner {
	return &DispatchRunner{local: local, remote: remote}
}

// Start dispatches the request to the local or remote runner.
func (d *DispatchRunner) Start(ctx context.Context, req port.TerminalStartRequest) (port.TerminalSession, error) {
	if req.RemoteHost == "" {
		return d.local.Start(ctx, req)
	}
	return d.remote.Start(ctx, req)
}
