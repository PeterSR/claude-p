package claudep

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	client "github.com/PeterSR/pupptyeer/clients/go"

	"github.com/PeterSR/claude-p/pkg/claudepty"
)

// newBackend selects and opens the pty backend for a run: the in-process pty
// (default, one-shot) or a pupptyeer daemon (persistent, multi-turn). reused
// reports whether an alive daemon session was continued (the in-process path
// is never reused).
func newBackend(opts Options, sessionID string, launch claudepty.ClaudeLaunch) (sess claudepty.PTYSession, reused bool, err error) {
	if !opts.PupptyeerDaemon {
		s, err := claudepty.StartInproc(launch)
		return s, false, err
	}

	sock := opts.PupptyeerSocket
	if sock == "" {
		sock = client.DefaultSocketPath()
	}
	if err := ensureDaemon(sock, opts); err != nil {
		return nil, false, err
	}

	// Continuation: if claude already has a transcript for this id but no live
	// daemon session is holding it, boot a fresh claude with --resume so the
	// conversation is reloaded. If a live session exists, EnsureSession reuses
	// it and these launch args are ignored.
	if claudepty.JSONLPath(sessionID) != "" {
		launch.Resume = sessionID
		launch.SessionID = ""
	}
	return claudepty.OpenDaemon(sock, launch, sessionID)
}

// ensureDaemon returns nil if a daemon is reachable at sock. If not, and a
// pupptyeer binary is available (PupptyeerBin, then $PUPPTYEER_BIN, then
// "pupptyeer" on PATH), it starts one detached and waits briefly for the socket.
func ensureDaemon(sock string, opts Options) error {
	if daemonReachable(sock) {
		return nil
	}
	bin := opts.PupptyeerBin
	if bin == "" {
		bin = os.Getenv("PUPPTYEER_BIN")
	}
	if bin == "" {
		if p, err := exec.LookPath("pupptyeer"); err == nil {
			bin = p
		}
	}
	if bin == "" {
		return fmt.Errorf("claudep: no pupptyeer daemon reachable at %s and no pupptyeer binary found (set --pupptyeer-bin or $PUPPTYEER_BIN, or start `pupptyeer daemon`)", sock)
	}

	cmd := exec.Command(bin, "daemon")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("claudep: failed to start pupptyeer daemon via %s: %w", bin, err)
	}
	// Don't wait on it — it's a long-lived daemon. Release our handle.
	_ = cmd.Process.Release()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if daemonReachable(sock) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("claudep: started pupptyeer daemon via %s but socket %s never came up", bin, sock)
}

func daemonReachable(sock string) bool {
	c, err := net.DialTimeout("unix", sock, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
