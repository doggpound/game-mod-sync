// Package tui is the full-screen interface (Bubble Tea). One screen runs for
// the whole session; each question the app asks (ui.UI) becomes a view on it,
// with the messages since the last question above it (PgUp/PgDn scroll them)
// and a spinner with what's happening while the app works. So there's no
// flicker between dialogs. When it ends, the last messages are printed to the
// normal terminal.
package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/doggpound/game-mod-sync/internal/ui"
)

// TUI implements ui.UI on a full screen.
type TUI struct {
	p    *tea.Program
	out  io.Writer
	done chan struct{}
	mu   sync.Mutex
	last []string // messages since the last question, printed at the end
}

// Usable reports whether in and out are a terminal the full screen can use.
func Usable(in io.Reader, out io.Writer) bool {
	fi, ok1 := in.(*os.File)
	fo, ok2 := out.(*os.File)
	return ok1 && ok2 && term.IsTerminal(int(fi.Fd())) && term.IsTerminal(int(fo.Fd()))
}

// Start opens the full screen.
func Start(in io.Reader, out io.Writer, version string) *TUI {
	t := &TUI{out: out, done: make(chan struct{})}
	t.p = tea.NewProgram(newModel(version), tea.WithAltScreen(), tea.WithInput(in), tea.WithOutput(out))
	go func() {
		_, _ = t.p.Run()
		close(t.done)
	}()
	return t
}

// Stop closes the full screen and prints the last messages.
func (t *TUI) Stop() {
	t.p.Quit()
	<-t.done
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.last) > 0 {
		fmt.Fprintln(t.out, strings.Join(t.last, "\n"))
	}
}

func (t *TUI) remember(kind, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch kind {
	case "warn":
		msg = "warning: " + msg
	case "info":
		msg = "==> " + msg
	}
	t.last = append(t.last, msg)
}

func (t *TUI) forget() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.last = nil
}

func (t *TUI) send(msg tea.Msg) { t.p.Send(msg) }

func (t *TUI) Info(msg string) { t.send(statusMsg(msg)) }
func (t *TUI) Say(msg string)  { t.remember("", msg); t.send(logMsg{"", msg}) }
func (t *TUI) OK(msg string)   { t.remember("ok", msg); t.send(logMsg{"ok", msg}) }
func (t *TUI) Warn(msg string) {
	t.remember("warn", msg)
	t.send(logMsg{"warn", "warning: " + msg})
}

// wait returns the answer, or false when the screen was closed (Ctrl+C).
func wait[T any](t *TUI, c chan T) (T, bool) {
	select {
	case v := <-c:
		t.forget()
		return v, true
	case <-t.done:
		var zero T
		return zero, false
	}
}

func (t *TUI) Choose(m ui.Menu) (int, string, error) {
	c := make(chan chooseRes, 1)
	t.send(chooseReq{m, c})
	r, ok := wait(t, c)
	if !ok {
		return 0, "", ui.ErrExit
	}
	return r.row, r.key, r.err
}

func (t *TUI) Confirm(q string, defYes bool) bool {
	c := make(chan bool, 1)
	t.send(confirmReq{q, defYes, c})
	r, ok := wait(t, c)
	return ok && r
}

func (t *TUI) Ask(q, def string) (string, error) {
	c := make(chan askRes, 1)
	t.send(askReq{q: q, def: def, reply: c})
	r, ok := wait(t, c)
	if !ok {
		return "", ui.ErrExit
	}
	return r.s, r.err
}

func (t *TUI) Password(q string) (string, error) {
	c := make(chan askRes, 1)
	t.send(askReq{q: q, password: true, reply: c})
	r, ok := wait(t, c)
	if !ok {
		return "", ui.ErrExit
	}
	return r.s, r.err
}

func (t *TUI) Check(cl ui.Checklist) ([]bool, error) {
	c := make(chan checkRes, 1)
	t.send(checkReq{cl, c})
	r, ok := wait(t, c)
	if !ok {
		return nil, ui.ErrExit
	}
	return r.on, r.err
}

func (t *TUI) View(title, text string) {
	c := make(chan struct{}, 1)
	t.send(viewReq{title, text, c})
	wait(t, c)
}
