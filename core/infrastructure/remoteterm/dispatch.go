package remoteterm

import (
	"context"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// Compile-time interface assertion.
var _ port.TerminalRunner = (*DispatchRunner)(nil)

// DispatchRunner routes TerminalStartRequests by RemoteHost:
//
//   - empty host        → local runner (PTY on this machine)
//   - "ssh://…" host     → ssh runner (PTY on a remote machine via its sshd)
//   - any other host    → ws runner (terminal on another multi-terminals instance)
type DispatchRunner struct {
	local port.TerminalRunner
	ws    port.TerminalRunner
	ssh   port.TerminalRunner
}

// NewDispatchRunner returns a DispatchRunner over the local, WebSocket and SSH
// runners.
func NewDispatchRunner(local, ws, ssh port.TerminalRunner) *DispatchRunner {
	return &DispatchRunner{local: local, ws: ws, ssh: ssh}
}

// Start dispatches the request to the local, ssh or ws runner by RemoteHost.
func (d *DispatchRunner) Start(ctx context.Context, req port.TerminalStartRequest) (port.TerminalSession, error) {
	switch {
	case req.RemoteHost == "":
		return d.local.Start(ctx, req)
	case IsSSHHost(req.RemoteHost):
		return d.ssh.Start(ctx, req)
	default:
		return d.ws.Start(ctx, req)
	}
}
