package claudep

import (
	"context"
	"fmt"
	"time"

	client "github.com/PeterSR/pupptyeer/clients/go"

	"github.com/PeterSR/claude-p/pkg/claudepty"
)

// ConnectDaemon dials the pupptyeer daemon in claude-p's namespace. socket is an
// optional override; empty uses the standard resolution ($PUPPTYEER_SOCK, then
// the per-user default). It connects-or-fails without ever spawning the daemon
// (lifecycle is a supervisor concern), and the returned client is safe to share
// across goroutines and sessions.
func ConnectDaemon(socket string) (*client.Client, error) {
	connOpts := []client.ConnectOption{client.WithNamespace(PupptyeerNamespace)}
	if socket != "" {
		connOpts = append(connOpts, client.WithSocket(socket))
	}
	return client.Connect(connOpts...)
}

// LaunchDaemon ensures a daemon session for opts exists and is sitting at the
// input prompt, addressed purely by id. It holds no per-session state and never
// attaches: a fresh `claude` is spawned in the daemon (or an already-warm one
// continued), driven past the trust/style modals, and its id returned. reused
// reports a continued warm session (which is already at the prompt). Pair the id
// with DaemonSession to drive the conversation.
func LaunchDaemon(ctx context.Context, c *client.Client, opts Options) (sessionID string, reused bool, err error) {
	applyDefaults(&opts)
	sessionID = resolveSessionID(opts)
	cwd := resolveCwd(opts)
	launch := buildLaunch(opts, sessionID, cwd)

	// Continuation: if claude already has a transcript for this id but no live
	// daemon session is holding it, boot with --resume so the conversation is
	// reloaded. If a live session exists, EnsureDaemonSession reuses it and these
	// launch args are ignored.
	if claudepty.JSONLPath(sessionID) != "" {
		launch.Resume = sessionID
		launch.SessionID = ""
	}

	reused, err = claudepty.EnsureDaemonSession(c, launch, sessionID)
	if err != nil {
		return "", false, fmt.Errorf("claudep: %w", err)
	}

	// A continued session is already past the modals and at the prompt; only a
	// freshly spawned one needs the ready wait.
	if !reused {
		ref := claudepty.NewDaemonRef(c, sessionID)
		if werr := claudepty.WaitForReady(ctx, ref, 45*time.Second); werr != nil {
			scr, _ := ref.CaptureScreen(0, 500*time.Millisecond)
			screen := scr.Text()
			if failure := claudepty.ClassifyInteractiveFailure(screen); failure != "" {
				return "", false, fmt.Errorf("claudep: %s (%w)", failure, werr)
			}
			return "", false, fmt.Errorf("claudep: claude never reached input prompt: %w", werr)
		}
	}
	return sessionID, reused, nil
}

// DaemonSession returns a connection-light Session for an existing daemon
// session id over the shared client. It attaches nothing and owns nothing, so
// it is cheap to construct one per operation (prompt, screen, keys, response)
// rather than holding it. cwd/socket are carried only for reporting and the
// pupptyeer monitor hint; pass "" when unknown.
func DaemonSession(c *client.Client, sessionID, cwd, socket string) *Session {
	return &Session{
		sess:      claudepty.NewDaemonRef(c, sessionID),
		sessionID: sessionID,
		cwd:       cwd,
		daemon:    true,
		socket:    socket,
		jsonlPath: claudepty.JSONLPath(sessionID),
	}
}

// DaemonSessionInfo is a daemon session as the driver reports it (a thin
// projection of pupptyeer's SessionInfo onto the fields a claude driver cares
// about).
type DaemonSessionInfo struct {
	SessionID    string `json:"session_id"`
	Cwd          string `json:"cwd,omitempty"`
	Alive        bool   `json:"alive"`
	LastActivity string `json:"last_activity,omitempty"`
}

// ListDaemon returns the claude-p sessions the daemon currently holds. Because
// the daemon is the source of truth, this surfaces sessions left warm by a prior
// run or created by another client - not just ones the caller launched.
func ListDaemon(c *client.Client) ([]DaemonSessionInfo, error) {
	infos, err := c.ListSessions()
	if err != nil {
		return nil, err
	}
	out := make([]DaemonSessionInfo, 0, len(infos))
	for _, i := range infos {
		out = append(out, DaemonSessionInfo{
			SessionID:    i.ID,
			Cwd:          i.Cwd,
			Alive:        i.Alive,
			LastActivity: i.LastActivity,
		})
	}
	return out, nil
}
