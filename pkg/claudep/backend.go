package claudep

import (
	client "github.com/PeterSR/pupptyeer/clients/go"

	"github.com/PeterSR/claude-p/pkg/claudepty"
)

// PupptyeerNamespace isolates the sessions claude-p creates inside the shared
// pupptyeer daemon (kubectl-context style). Sessions other apps create live in
// their own namespaces and stay invisible to claude-p's list/kill/gc, and vice
// versa.
const PupptyeerNamespace = "claude-p"

// newBackend selects and opens the pty backend for a run: the in-process pty
// (default, one-shot) or a pupptyeer daemon (persistent, multi-turn). reused
// reports whether an alive daemon session was continued (the in-process path
// is never reused).
func newBackend(opts Options, sessionID string, launch claudepty.ClaudeLaunch) (sess claudepty.PTYSession, reused bool, err error) {
	if !opts.PupptyeerDaemon {
		s, err := claudepty.StartInproc(launch)
		return s, false, err
	}

	// Connect-or-scream lives in the pupptyeer client: it resolves the default
	// socket (or uses --pupptyeer-socket) and, on an unreachable daemon, returns
	// one canonical, actionable error WITHOUT ever spawning anything. claude-p
	// does not manage the daemon lifecycle: that is a supervisor/system-package
	// concern (think systemd/launchd). Sessions are tagged with claude-p's
	// namespace so they stay isolated from other apps sharing the daemon.
	connOpts := []client.ConnectOption{client.WithNamespace(PupptyeerNamespace)}
	if opts.PupptyeerSocket != "" {
		connOpts = append(connOpts, client.WithSocket(opts.PupptyeerSocket))
	}
	c, err := client.Connect(connOpts...)
	if err != nil {
		return nil, false, err
	}

	// Continuation: if claude already has a transcript for this id but no live
	// daemon session is holding it, boot a fresh claude with --resume so the
	// conversation is reloaded. If a live session exists, OpenDaemon reuses
	// it and these launch args are ignored.
	if claudepty.JSONLPath(sessionID) != "" {
		launch.Resume = sessionID
		launch.SessionID = ""
	}
	return claudepty.OpenDaemon(c, launch, sessionID)
}
