package claudepty

import (
	"context"
	"fmt"
	"time"

	client "github.com/PeterSR/pupptyeer/clients/go"
)

// DaemonRef is a connection-light PTYSession over a pupptyeer daemon session
// addressed purely by id. Unlike the handle OpenDaemon returns, it never
// attaches and runs no drain goroutine: every operation is a stateless
// request/reply on a shared client, so the same daemon session can be driven by
// id from anywhere without per-session bookkeeping. Exit is observed by asking
// the daemon (ListSessions) rather than by watching an attached event stream.
//
// Close is a no-op: a DaemonRef owns neither the session (the daemon does) nor
// the client (the caller does). The shared client is safe for concurrent use —
// the pupptyeer client correlates replies by request id — so one client can back
// many DaemonRefs at once.
type DaemonRef struct {
	c       *client.Client
	session string
}

// NewDaemonRef wraps a shared, already-connected client and a session id as a
// PTYSession. The client must be connected in the session's namespace; the ref
// does not close it.
func NewDaemonRef(c *client.Client, sessionID string) *DaemonRef {
	return &DaemonRef{c: c, session: sessionID}
}

func (d *DaemonRef) WriteInput(p []byte) error { return d.c.WritePane(d.session, p) }

func (d *DaemonRef) CaptureScreen(settle, timeout time.Duration) (*Screen, error) {
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

// alive reports whether the daemon still lists this session as running. On a
// list error it assumes alive rather than declaring a false exit.
func (d *DaemonRef) alive() bool {
	infos, err := d.c.ListSessions()
	if err != nil {
		return true
	}
	for _, info := range infos {
		if info.ID == d.session {
			return info.Alive
		}
	}
	return false
}

func (d *DaemonRef) Exited() (bool, int) {
	if d.alive() {
		return false, 0
	}
	return true, 0
}

func (d *DaemonRef) Wait(ctx context.Context) (int, error) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		if exited, code := d.Exited(); exited {
			return code, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (d *DaemonRef) Kill() error  { return d.c.Kill(d.session) }
func (d *DaemonRef) Close() error { return nil }

// EnsureDaemonSession makes sure a daemon session with id sessionID exists,
// continuing an already-alive one (reused=true) or spawning a fresh `claude`
// with the given launch flags (reused=false). Unlike OpenDaemon it neither
// attaches nor takes ownership of the client — it is the stateless launch
// primitive behind a driver that addresses sessions purely by id. All calls use
// the client's connection-default namespace.
func EnsureDaemonSession(c *client.Client, l ClaudeLaunch, sessionID string) (reused bool, err error) {
	bin := l.Binary
	if bin == "" {
		bin = "claude"
	}
	env := l.Env
	if env == nil {
		env = SubscriptionEnv()
	}
	infos, err := c.ListSessions()
	if err != nil {
		return false, err
	}
	for _, info := range infos {
		if info.ID == sessionID && info.Alive {
			return true, nil
		}
	}
	// Drive new_session directly (not the client's EnsureSession helper) so we
	// can verify the daemon honored our requested id — everything downstream
	// (capture, the JSONL transcript, continuation) is keyed on claude's
	// --session-id, so a mismatched pty id would break the very next call.
	got, nerr := c.NewSession(bin, BuildClaudeArgs(l), l.Cwd, envSliceToMap(env), VTCols, VTRows,
		client.WithSessionID(sessionID), client.WithGetOrCreate())
	if nerr != nil {
		return false, nerr
	}
	if got != sessionID {
		_ = c.Kill(got)
		return false, fmt.Errorf("pupptyeer daemon did not honor the requested session id (created %q, wanted %q): needs pupptyeer >= 0.9.0; upgrade it or stop a stale older daemon holding the socket", got, sessionID)
	}
	return false, nil
}
