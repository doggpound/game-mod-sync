package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/doggpound/game-mod-sync/internal/ui"
)

// press feeds keys to the model, the way Bubble Tea would.
func press(t *testing.T, m tea.Model, keys ...string) tea.Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		case "pgup":
			msg = tea.KeyMsg{Type: tea.KeyPgUp}
		case "ctrl+c":
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m, _ = m.Update(msg)
	}
	return m
}

func start(t *testing.T, msgs ...tea.Msg) tea.Model {
	t.Helper()
	var m tea.Model = newModel("1.0.0")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func menu() ui.Menu {
	return ui.Menu{Title: "Main menu", Question: "Which profile?", Rows: []string{"Valheim / MyProfile", "Valheim / Vanilla", "Lethal / Fun"},
		Keys: []ui.Key{{Letter: 'e', Name: "Edit"}}, NoBack: true, Help: "Main menu help text"}
}

func TestMenu(t *testing.T) {
	c := make(chan chooseRes, 1)
	m := start(t, logMsg{"ok", "Synced."}, chooseReq{menu(), c})
	v := m.View()
	for _, want := range []string{"Main menu", "Which profile?", "❯ Valheim / MyProfile", "Synced.", "e edit", "esc/q exit"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	m = press(t, m, "down", "down", "down", "enter") // wraps to the first row
	if r := <-c; r.row != 0 || r.err != nil {
		t.Errorf("got %+v", r)
	}
	if v := m.View(); strings.Contains(v, "Synced.") {
		t.Error("the log wasn't cleared after the answer")
	}
	m = press(t, start(t, chooseReq{menu(), c}), "down", "e")
	if r := <-c; r.row != 1 || r.key != "Edit" {
		t.Errorf("row key: %+v", r)
	}
	press(t, start(t, chooseReq{menu(), c}), "3", "enter")
	if r := <-c; r.row != 2 {
		t.Errorf("number: %+v", r)
	}
	press(t, start(t, chooseReq{menu(), c}), "esc")
	if r := <-c; !errors.Is(r.err, ui.ErrExit) {
		t.Errorf("esc on the main menu: %+v", r)
	}
	sub := menu()
	sub.NoBack = false
	press(t, start(t, chooseReq{sub, c}), "esc")
	if r := <-c; !errors.Is(r.err, ui.ErrBack) {
		t.Errorf("esc on a sub menu: %+v", r)
	}
	m = press(t, start(t, chooseReq{menu(), c}), "?")
	if !strings.Contains(m.View(), "Main menu help text") {
		t.Errorf("help not shown:\n%s", m.View())
	}
	m = press(t, m, "x", "enter") // any key closes the help; then Enter answers
	if r := <-c; r.row != 0 {
		t.Errorf("after help: %+v", r)
	}
}

func TestConfirm(t *testing.T) {
	c := make(chan bool, 1)
	press(t, start(t, confirmReq{"Apply?", false, c}), "enter")
	if <-c {
		t.Error("default No answered yes")
	}
	press(t, start(t, confirmReq{"Leave it alone?", true, c}), "enter")
	if !<-c {
		t.Error("default Yes answered no")
	}
	press(t, start(t, confirmReq{"Apply?", false, c}), "right", "enter")
	if !<-c {
		t.Error("moving to Yes didn't answer yes")
	}
	press(t, start(t, confirmReq{"Apply?", true, c}), "esc")
	if <-c {
		t.Error("esc answered yes")
	}
	press(t, start(t, confirmReq{"Apply?", false, c}), "y")
	if !<-c {
		t.Error("y didn't answer yes")
	}
}

func TestAsk(t *testing.T) {
	c := make(chan askRes, 1)
	press(t, start(t, askReq{q: "Port", def: "22", reply: c}), "enter")
	if r := <-c; r.s != "22" {
		t.Errorf("default: %+v", r)
	}
	press(t, start(t, askReq{q: "Server", reply: c}), "g", "a", "m", "e", "enter")
	if r := <-c; r.s != "game" {
		t.Errorf("typed: %+v", r)
	}
	m := press(t, start(t, askReq{q: "Password", password: true, reply: c}), "s", "e", "c")
	if v := m.View(); strings.Contains(v, "sec") || !strings.Contains(v, "•••") {
		t.Errorf("password shown:\n%s", v)
	}
	press(t, m, "enter")
	if r := <-c; r.s != "sec" {
		t.Errorf("password: %+v", r)
	}
	press(t, start(t, askReq{q: "Server", reply: c}), "esc")
	if r := <-c; !errors.Is(r.err, ui.ErrBack) {
		t.Errorf("esc: %+v", r)
	}
}

func checklist() ui.Checklist {
	return ui.Checklist{Title: "Config files", Question: "Ticked files are copied.", Header: "STATUS   FILE",
		Rows:    []string{"new      a.cfg", "differs  b.cfg", "differs  c.cfg"},
		On:      []bool{true, false, false},
		Details: []string{"", "    Range\n        server: 10\n        yours:  20", ""}}
}

func TestCheck(t *testing.T) {
	c := make(chan checkRes, 1)
	m := start(t, checkReq{checklist(), c})
	v := m.View()
	for _, want := range []string{"Config files", "STATUS   FILE", "[x]", "▸", "space tick"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "server: 10") {
		t.Error("details shown before opening the row")
	}
	m = press(t, m, "down", "right")
	if !strings.Contains(m.View(), "server: 10") {
		t.Errorf("details not shown after Right:\n%s", m.View())
	}
	m = press(t, m, "space", "left", "enter")
	if r := <-c; r.err != nil || len(r.on) != 3 || !r.on[0] || !r.on[1] || r.on[2] {
		t.Errorf("ticks: %+v", r)
	}
	press(t, start(t, checkReq{checklist(), c}), "n", "enter")
	if r := <-c; r.on[0] {
		t.Errorf("none: %+v", r)
	}
	press(t, start(t, checkReq{checklist(), c}), "a", "enter")
	if r := <-c; !r.on[0] || !r.on[1] || !r.on[2] {
		t.Errorf("all: %+v", r)
	}
	press(t, start(t, checkReq{checklist(), c}), "space", "esc")
	if r := <-c; !errors.Is(r.err, ui.ErrBack) {
		t.Errorf("esc: %+v", r)
	}
}

func TestViewAndLog(t *testing.T) {
	c := make(chan struct{}, 1)
	m := start(t, viewReq{"Differences", "line one\nline two", c})
	if v := m.View(); !strings.Contains(v, "line two") || !strings.Contains(v, "Differences") {
		t.Errorf("view:\n%s", v)
	}
	press(t, m, "enter")
	<-c

	// A long log shows its end, and PgUp scrolls back to the start.
	var msgs []tea.Msg
	for i := 1; i <= 60; i++ {
		msgs = append(msgs, logMsg{"", "log line " + strings.Repeat("x", i%3) + string(rune('A'+i%26))})
	}
	ch := make(chan bool, 1)
	msgs = append(msgs, logMsg{"", "THE END"}, confirmReq{"Apply?", false, ch})
	m = start(t, msgs...)
	if v := m.View(); !strings.Contains(v, "THE END") || !strings.Contains(v, "more line(s) - PgUp") {
		t.Errorf("log end:\n%s", v)
	}
	if lines := strings.Count(m.View(), "\n") + 1; lines > 30 {
		t.Errorf("the view is %d lines, taller than the screen", lines)
	}
	m = press(t, m, "pgup", "pgup", "pgup")
	if v := m.View(); strings.Contains(v, "THE END") {
		t.Errorf("PgUp didn't scroll:\n%s", v)
	}
}

func TestBusyAndQuit(t *testing.T) {
	m := start(t, statusMsg("Comparing with the server ..."))
	if !strings.Contains(m.View(), "Comparing with the server ...") {
		t.Errorf("status:\n%s", m.View())
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil || m.View() != "" {
		t.Error("ctrl+c didn't quit")
	}
}
