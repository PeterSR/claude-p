package claudepty

import (
	"context"
	"time"

	"github.com/PeterSR/pupptyeer/pkg/ptysession"
)

// inprocSession drives claude in an in-process pty via pupptyeer's
// program-agnostic core. No external binary, no daemon — this is the default
// one-shot path.
type inprocSession struct {
	core *ptysession.Session
}

// StartInproc spawns claude in an in-process pty sized VTCols x VTRows with the
// configured launch flags and (by default) a subscription-only env.
func StartInproc(l ClaudeLaunch) (PTYSession, error) {
	bin := l.Binary
	if bin == "" {
		bin = "claude"
	}
	env := l.Env
	if env == nil {
		env = SubscriptionEnv()
	}
	core, err := ptysession.Start(ptysession.Config{
		Command: bin,
		Args:    BuildClaudeArgs(l),
		Cwd:     l.Cwd,
		Env:     env,
		Cols:    VTCols,
		Rows:    VTRows,
	})
	if err != nil {
		return nil, err
	}
	return &inprocSession{core: core}, nil
}

func (b *inprocSession) WriteInput(p []byte) error { return b.core.Write(p) }

func (b *inprocSession) CaptureScreen(settle, timeout time.Duration) (*Screen, error) {
	g, err := b.core.CaptureScreen(settle, timeout)
	if err != nil {
		return nil, err
	}
	return fromCoreScreen(g), nil
}

func (b *inprocSession) Wait(ctx context.Context) (int, error) { return b.core.Wait(ctx) }
func (b *inprocSession) Exited() (bool, int)                   { return b.core.Exited() }
func (b *inprocSession) Kill() error                           { b.core.Kill(); return nil }
func (b *inprocSession) Close() error                          { return b.core.Close() }

func fromCoreScreen(g *ptysession.Screen) *Screen {
	return &Screen{
		Cols:      g.Cols,
		Rows:      g.Rows,
		Lines:     g.Lines,
		Cursor:    Cursor{Row: g.Cursor.Row, Col: g.Cursor.Col, Visible: g.Cursor.Visible},
		AltScreen: g.AltScreen,
	}
}
