package remoteterm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// Compile-time interface assertions.
var _ port.TerminalRunner = (*SSHRunner)(nil)
var _ port.TerminalSession = (*sshSession)(nil)

// SSHScheme is the RemoteHost prefix that selects SSH transport. A pane whose
// remote host starts with this connects to an ordinary OpenSSH server (sshd) on
// the target machine rather than to another multi-terminals instance, so no
// second multi-terminals process — and no authorized-keys setup — is needed on
// the far side.
const SSHScheme = "ssh://"

// sshDefaultPort is used when an ssh:// host omits the port.
const sshDefaultPort = "22"

// sshInsecureEnv, when set to a non-empty value, disables SSH host-key
// verification (accepts any host key). Use only on trusted networks (LAN/VPN)
// where you cannot pre-populate known_hosts.
const sshInsecureEnv = "MULTI_TERMINALS_SSH_INSECURE"

// IsSSHHost reports whether a RemoteHost value selects the SSH transport.
func IsSSHHost(remoteHost string) bool {
	return strings.HasPrefix(strings.TrimSpace(remoteHost), SSHScheme)
}

// SSHRunner is a port.TerminalRunner that starts terminal sessions on a remote
// machine over SSH by connecting to its sshd (identified by
// TerminalStartRequest.RemoteHost in "ssh://[user@]host[:port]" form). The
// shell runs on the remote machine inside an allocated PTY; its output is
// streamed back over the SSH channel.
//
// Credentials come from the local user's existing SSH setup — the SSH agent
// ($SSH_AUTH_SOCK) and the default identity files under ~/.ssh — so no
// key management is added to multi-terminals itself. Host keys are verified
// against ~/.ssh/known_hosts unless MULTI_TERMINALS_SSH_INSECURE is set.
type SSHRunner struct {
	// authMethods returns the SSH auth methods to try for the given login user,
	// plus a cleanup function to release any resources they hold (e.g. the
	// ssh-agent socket) once the handshake is done. nil selects the production
	// default (agent + ~/.ssh identity files). Overridable in tests.
	authMethods func(user string) ([]ssh.AuthMethod, func())
	// hostKeyCallback verifies the server host key. nil selects the production
	// default (known_hosts, or insecure when MULTI_TERMINALS_SSH_INSECURE is
	// set). Overridable in tests.
	hostKeyCallback ssh.HostKeyCallback
	// timeout bounds the TCP dial and SSH handshake.
	timeout time.Duration
}

// NewSSHRunner returns an SSHRunner using the local user's SSH agent and
// ~/.ssh identity files for authentication and ~/.ssh/known_hosts for host-key
// verification.
func NewSSHRunner() *SSHRunner {
	return &SSHRunner{timeout: 15 * time.Second}
}

// Start opens an SSH connection to req.RemoteHost, requests a PTY, starts the
// remote login shell and returns a port.TerminalSession bridged over the SSH
// channel. The context bounds connection setup only; the returned session
// outlives it and is terminated solely by Close.
func (r *SSHRunner) Start(ctx context.Context, req port.TerminalStartRequest) (port.TerminalSession, error) {
	login, addr, err := parseSSHTarget(req.RemoteHost)
	if err != nil {
		return nil, fmt.Errorf("remote terminal (ssh): %w", err)
	}

	authFn := r.authMethods
	if authFn == nil {
		authFn = defaultSSHAuthMethods
	}
	methods, cleanup := authFn(login)
	// The auth methods may hold an open ssh-agent socket; it is only needed
	// through the handshake (which completes before Start returns), so release
	// it on return to avoid leaking a file descriptor per session.
	defer cleanup()
	if len(methods) == 0 {
		return nil, fmt.Errorf("remote terminal (ssh): no SSH credentials available: load a key into ssh-agent (ssh-add), or create a default key under ~/.ssh (id_ed25519 / id_ecdsa / id_rsa)")
	}

	hostKeyCB := r.hostKeyCallback
	if hostKeyCB == nil {
		hostKeyCB, err = resolveHostKeyCallback()
		if err != nil {
			return nil, fmt.Errorf("remote terminal (ssh): %w", err)
		}
	}

	cfg := &ssh.ClientConfig{
		User:            login,
		Auth:            methods,
		HostKeyCallback: hostKeyCB,
		Timeout:         r.timeout,
	}

	client, err := dialSSH(ctx, addr, cfg, r.timeout)
	if err != nil {
		return nil, fmt.Errorf("remote terminal (ssh): connect %s@%s: %w", login, addr, hintSSHError(err))
	}

	sess, err := startSSHSession(client, req)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("remote terminal (ssh): %s@%s: %w", login, addr, err)
	}
	return sess, nil
}

// dialSSH performs a context-bounded TCP dial followed by the SSH handshake.
// The context governs setup only; once the client is returned it is detached
// from ctx so the session can outlive the originating request.
func dialSSH(ctx context.Context, addr string, cfg *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	// The SSH handshake can block; enforce the timeout on the raw connection so
	// a silent peer cannot hang Start forever.
	_ = conn.SetDeadline(time.Now().Add(timeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	// Clear the setup deadline: interactive sessions read/write indefinitely.
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

// startSSHSession allocates a PTY on an SSH channel and starts the remote
// shell, returning the bridged session.
func startSSHSession(client *ssh.Client, req port.TerminalStartRequest) (*sshSession, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	cols, rows := req.Cols, req.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", int(rows), int(cols), modes); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("request pty: %w", err)
	}

	// An explicit shell/command runs via Start; otherwise the remote login
	// shell is launched. Panes normally carry no shell (the OS default is used),
	// so most sessions land in the user's login shell on the remote machine.
	if req.Shell != "" {
		err = session.Start(req.Shell)
	} else {
		err = session.Shell()
	}
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	s := &sshSession{
		id:      req.SessionID,
		client:  client,
		session: session,
		stdin:   stdin,
		stdout:  stdout,
		out:     make(chan []byte, 256),
		done:    make(chan struct{}),
	}
	go s.pump()
	return s, nil
}

// parseSSHTarget parses an "ssh://[user@]host[:port]" RemoteHost value into the
// login user and the "host:port" dial address. A missing port defaults to 22;
// a missing user defaults to the current OS user.
func parseSSHTarget(remoteHost string) (login, addr string, err error) {
	raw := strings.TrimSpace(remoteHost)
	if !strings.HasPrefix(raw, SSHScheme) {
		return "", "", fmt.Errorf("remote host must start with %q", SSHScheme)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid ssh url %q: %w", remoteHost, err)
	}
	host := u.Hostname()
	if host == "" {
		return "", "", fmt.Errorf("ssh remote host %q has no host name", remoteHost)
	}
	// Only "ssh://[user@]host[:port]" is meaningful here; a path, query or
	// fragment means the input is malformed. Reject it rather than silently
	// connecting to just the host and dropping the extra components.
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", "", fmt.Errorf("ssh remote host %q must be of the form ssh://[user@]host[:port] (no path/query/fragment)", remoteHost)
	}
	port := u.Port()
	if port == "" {
		port = sshDefaultPort
	}
	login = u.User.Username()
	if login == "" {
		login, err = currentUsername()
		if err != nil {
			return "", "", fmt.Errorf("ssh remote host %q has no user@ and the current user is unknown: %w", remoteHost, err)
		}
	}
	return login, net.JoinHostPort(host, port), nil
}

// currentUsername returns the current OS login name, falling back to the USER /
// USERNAME environment variables when os/user is unavailable (some static or
// cross-compiled builds).
func currentUsername() (string, error) {
	if u, err := user.Current(); err == nil && u.Username != "" {
		// On Windows the name may be "DOMAIN\\user"; keep only the account part.
		name := u.Username
		if i := strings.LastIndexAny(name, `\/`); i >= 0 {
			name = name[i+1:]
		}
		return name, nil
	}
	for _, k := range []string{"USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return v, nil
		}
	}
	return "", errors.New("no current user; specify one as ssh://user@host")
}

// defaultSSHAuthMethods assembles auth methods from the local user's existing
// SSH setup: the running ssh-agent first, then the default identity files under
// ~/.ssh. Encrypted key files without a passphrase in the agent are skipped.
// The returned cleanup function closes any resources the methods hold (the
// ssh-agent socket) and must be called once the handshake has completed.
func defaultSSHAuthMethods(_ string) ([]ssh.AuthMethod, func()) {
	var methods []ssh.AuthMethod
	var closers []func() error
	if m, closer := agentAuthMethod(); m != nil {
		methods = append(methods, m)
		if closer != nil {
			closers = append(closers, closer)
		}
	}
	if signers := defaultIdentitySigners(); len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	cleanup := func() {
		for _, c := range closers {
			_ = c()
		}
	}
	return methods, cleanup
}

// agentAuthMethod returns an auth method backed by the SSH agent if one is
// reachable via $SSH_AUTH_SOCK, plus a closer for the agent socket. Both are
// nil when no agent is available. The caller must invoke the closer once the
// SSH handshake has completed so the socket is not leaked.
func agentAuthMethod() (ssh.AuthMethod, func() error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, nil
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, nil
	}
	ag := agent.NewClient(conn)
	return ssh.PublicKeysCallback(ag.Signers), conn.Close
}

// defaultIdentitySigners loads unencrypted default identity files from ~/.ssh.
// Encrypted keys (which would need an interactive passphrase) are skipped; use
// the SSH agent for those.
func defaultIdentitySigners() []ssh.Signer {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var signers []ssh.Signer
	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		data, err := os.ReadFile(filepath.Join(home, ".ssh", name))
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			// Encrypted or unparseable key: skip; agent auth may still work.
			continue
		}
		signers = append(signers, signer)
	}
	return signers
}

// resolveHostKeyCallback builds the production host-key verification callback:
// insecure (accept any) when MULTI_TERMINALS_SSH_INSECURE is set, otherwise
// verification against ~/.ssh/known_hosts.
func resolveHostKeyCallback() (ssh.HostKeyCallback, error) {
	if os.Getenv(sshInsecureEnv) != "" {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot locate home directory for known_hosts: %w", err)
	}
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	cb, err := knownhosts.New(khPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w; connect once with `ssh` to record the host, or set %s=1 on a trusted network",
			khPath, err, sshInsecureEnv)
	}
	return cb, nil
}

// hintSSHError augments an SSH connection error with actionable guidance for
// the common failure modes (unknown host key, auth rejected).
func hintSSHError(err error) error {
	var ke *knownhosts.KeyError
	if errors.As(err, &ke) {
		if len(ke.Want) == 0 {
			return fmt.Errorf("%w — host key not in known_hosts; run `ssh` to this host once to record it, or set %s=1 on a trusted network", err, sshInsecureEnv)
		}
		return fmt.Errorf("%w — host key changed (possible MITM); fix ~/.ssh/known_hosts if the host legitimately changed", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "unable to authenticate") || strings.Contains(msg, "no supported methods") {
		return fmt.Errorf("%w — authentication rejected; ensure your key is in the remote ~/.ssh/authorized_keys (ssh-copy-id) or loaded in your ssh-agent", err)
	}
	if strings.Contains(msg, "connection refused") {
		return fmt.Errorf("%w — nothing is listening on that port; is sshd running and the port correct? (default 22)", err)
	}
	return err
}

// sshSession implements port.TerminalSession over an SSH channel to a remote
// sshd.
type sshSession struct {
	id      string
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader

	out  chan []byte
	done chan struct{}

	writeMu   sync.Mutex // serializes stdin writes and window changes
	closeOnce sync.Once  // ensures the SSH resources are torn down once
	chanOnce  sync.Once  // ensures out/done are closed once (owned by pump)
}

// pump reads remote output and forwards it to the out channel. It exits when
// the channel reaches EOF (remote shell exited or the connection was closed by
// Close) and is the sole closer of out and done.
func (s *sshSession) pump() {
	buf := make([]byte, 4096)
	for {
		n, err := s.stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.out <- chunk
		}
		if err != nil {
			break
		}
	}
	s.chanOnce.Do(func() {
		close(s.out)
		// Reap the remote command and tear the connection down so no goroutine
		// or socket is leaked when the shell exits on its own.
		_ = s.session.Wait()
		s.teardown()
		close(s.done)
	})
}

// teardown closes the SSH session and underlying client exactly once.
func (s *sshSession) teardown() {
	s.closeOnce.Do(func() {
		_ = s.session.Close()
		_ = s.client.Close()
	})
}

// ID returns the session identifier.
func (s *sshSession) ID() string { return s.id }

// Write sends input bytes to the remote shell's standard input.
func (s *sshSession) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.stdin.Write(data)
	return err
}

// Resize updates the remote PTY window size via an SSH window-change request.
func (s *sshSession) Resize(cols, rows uint16) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.session.WindowChange(int(rows), int(cols))
}

// Output returns the read-only output channel, closed when the session ends.
func (s *sshSession) Output() <-chan []byte { return s.out }

// Done returns a channel closed when the session is fully terminated.
func (s *sshSession) Done() <-chan struct{} { return s.done }

// Close terminates the session by tearing down the SSH connection; the remote
// shell dies when its channel closes. The pump goroutine observes the resulting
// EOF and closes out/done. Idempotent.
func (s *sshSession) Close() error {
	s.teardown()
	return nil
}
