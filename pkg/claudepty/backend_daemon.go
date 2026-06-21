package claudepty

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	client "github.com/PeterSR/pupptyeer/clients/go"
)

// daemonSession drives claude that lives inside a pupptyeer daemon. The claude
// TUI process outlives any single claude-p invocation, so the next invocation
// (same session id) continues the same conversation without paying the
// TUI-startup cost again.
type daemonSession struct {
	c       *client.Client
	session string

	exited atomic.Bool
	code   atomic.Int32
	exitCh chan int

	closeOnce sync.Once
}

// OpenDaemon takes an already-connected pupptyeer client and ensures a session
// whose id is sessionID: if one is already alive it is continued (reused=true),
// else a fresh `claude` is launched with that id (reused=false). It attaches and
// begins draining events (required to learn the exit code and to keep capture
// calls from stalling on backpressure). OpenDaemon takes ownership of c: it
// closes c on every error path, and the returned session closes c when done.
// All session calls operate in c's connection-default namespace.
func OpenDaemon(c *client.Client, l ClaudeLaunch, sessionID string) (sess PTYSession, reused bool, err error) {
	bin := l.Binary
	if bin == "" {
		bin = "claude"
	}
	env := l.Env
	if env == nil {
		env = SubscriptionEnv()
	}
	// Continue an already-alive session with this id, else create one. We don't
	// use the client's EnsureSession helper because it discards the id the
	// daemon assigns: we must verify the daemon actually honored our requested
	// id (see below), so we drive new_session directly.
	infos, err := c.ListSessions()
	if err != nil {
		_ = c.Close()
		return nil, false, err
	}
	for _, info := range infos {
		if info.ID == sessionID && info.Alive {
			reused = true
			break
		}
	}
	if !reused {
		got, nerr := c.NewSession(bin, BuildClaudeArgs(l), l.Cwd, envSliceToMap(env), VTCols, VTRows,
			client.WithSessionID(sessionID), client.WithGetOrCreate())
		if nerr != nil {
			_ = c.Close()
			return nil, false, nerr
		}
		// A daemon that predates bring-your-own-id (pupptyeer < 0.6.0) silently
		// ignores requested_id and assigns its own UUID. We key everything
		// (attach/capture, the JSONL transcript, and continuation across runs)
		// on claude's --session-id, so a mismatched pty-session id would make
		// the very next Attach fail with "session not found". Fail clearly and
		// reap the orphan it spawned rather than limping on.
		if got != sessionID {
			_ = c.Kill(got)
			_ = c.Close()
			return nil, false, fmt.Errorf("pupptyeer daemon did not honor the requested session id (created %q, wanted %q): --pupptyeer-daemon needs pupptyeer >= 0.9.0; upgrade it, or stop a stale older `pupptyeer daemon` that is still holding the socket", got, sessionID)
		}
	}
	// Attach so the daemon delivers this session's exit event to us, then drain
	// events forever (an attached-but-undrained connection eventually stalls
	// its own request/reply calls, including CaptureScreen).
	if err := c.Attach(sessionID, VTCols, VTRows); err != nil {
		_ = c.Close()
		return nil, false, err
	}
	d := &daemonSession{c: c, session: sessionID, exitCh: make(chan int, 1)}
	go d.drain()
	return d, reused, nil
}

func (d *daemonSession) drain() {
	for m := range d.c.Events() {
		switch m.Type {
		case client.TypeExit:
			if m.ExitCode != nil {
				d.code.Store(int32(*m.ExitCode))
			}
			d.markExited()
		case client.TypeSessionClosed:
			d.markExited()
		}
	}
	// Events channel closed = the connection ended; treat as exited so waiters
	// don't block forever.
	d.markExited()
}

func (d *daemonSession) markExited() {
	d.exited.Store(true)
	select {
	case d.exitCh <- int(d.code.Load()):
	default:
	}
}

func (d *daemonSession) WriteInput(p []byte) error { return d.c.WritePane(d.session, p) }

func (d *daemonSession) CaptureScreen(settle, timeout time.Duration) (*Screen, error) {
	var opts []client.CaptureOption
	if settle > 0 {
		opts = append(opts, client.WithSettle(int(settle/time.Millisecond)))
	}
	if timeout > 0 {
		opts = append(opts, client.WithTimeout(int(timeout/time.Millisecond)))
	}
	scr, err := d.c.CaptureScreen(d.session, opts...)
	if err != nil {
		return nil, err
	}
	return &Screen{
		Cols:      scr.Cols,
		Rows:      scr.Rows,
		Lines:     scr.Lines,
		Cursor:    Cursor{Row: scr.Cursor.Row, Col: scr.Cursor.Col, Visible: scr.Cursor.Visible},
		AltScreen: scr.AltScreen,
	}, nil
}

func (d *daemonSession) Wait(ctx context.Context) (int, error) {
	if d.exited.Load() {
		return int(d.code.Load()), nil
	}
	select {
	case code := <-d.exitCh:
		return code, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (d *daemonSession) Exited() (bool, int) {
	if d.exited.Load() {
		return true, int(d.code.Load())
	}
	return false, 0
}

func (d *daemonSession) Kill() error { return d.c.Kill(d.session) }

// Close detaches and drops the connection but leaves the session ALIVE in the
// daemon for the next invocation to continue. Use Kill to actually end it.
func (d *daemonSession) Close() error {
	d.closeOnce.Do(func() {
		_ = d.c.Detach(d.session)
		_ = d.c.Close()
	})
	return nil
}

// envSliceToMap converts an exec-style ["K=V"] env to the map the daemon's
// new_session wire takes. Entries without '=' are skipped.
func envSliceToMap(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}
