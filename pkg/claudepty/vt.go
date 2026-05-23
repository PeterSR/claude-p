package claudepty

import (
	"io"
	"strings"

	"github.com/charmbracelet/x/vt"
)

// VTCols / VTRows define the virtual terminal we replay the captured
// pty bytes into. The size matters because claude's TUI layout
// decisions depend on the terminal dimensions it sees; pick something
// wide enough that the panel never wraps in ways the caller doesn't
// expect.
const (
	VTCols = 200
	VTRows = 60
)

// RenderVT replays a raw pty byte stream into a virtual terminal grid
// and returns the resulting text, row by row, trailing whitespace
// trimmed and visual gaps preserved as spaces.
//
// Why this matters versus stripping ANSI escapes:
//
//   - claude's TUI positions characters via cursor-move codes instead
//     of writing literal spaces. After plain ANSI stripping the words
//     "Current session" arrive as "Currentsession". The grid re-
//     introduces real spaces between visually-separated cells.
//   - claude redraws the screen continuously — status-line ticks,
//     welcome panel updates, spinner frames. A naive concat of the byte
//     stream accumulates every write ever performed, including stale
//     content. The grid only keeps the latest character at each cell.
func RenderVT(raw []byte) string {
	e := vt.NewEmulator(VTCols, VTRows)
	// The emulator writes ANSI responses (e.g. InBandResize replies) to
	// an internal io.Pipe. Without a reader those writes block Write()
	// forever. Drain into Discard until Close() releases the goroutine.
	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, e)
		close(drained)
	}()
	_, _ = e.Write(raw)
	_ = e.Close()
	<-drained

	var b strings.Builder
	for y := 0; y < VTRows; y++ {
		var row strings.Builder
		for x := 0; x < VTCols; x++ {
			cell := e.CellAt(x, y)
			if cell == nil {
				row.WriteByte(' ')
				continue
			}
			s := cell.String()
			if s == "" {
				row.WriteByte(' ')
				continue
			}
			row.WriteString(s)
		}
		b.WriteString(strings.TrimRight(row.String(), " "))
		b.WriteByte('\n')
	}
	return b.String()
}
