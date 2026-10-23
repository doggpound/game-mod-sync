package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/doggpound/game-mod-sync/internal/ui"
)

// Colours and styles.
var (
	accent   = lipgloss.AdaptiveColor{Light: "#5A3FC0", Dark: "#9D7BFF"}
	subtle   = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: "#8A8A8A"}
	barStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(accent).Padding(0, 1)
	dimStyle = lipgloss.NewStyle().Foreground(subtle)
	boldSt   = lipgloss.NewStyle().Bold(true)
	curStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	okStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"})
	warnSt   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"})
	infoSt   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0969DA", Dark: "#58A6FF"})
	keySt    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#444444", Dark: "#BBBBBB"})
	btnOn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(accent).Padding(0, 2)
	btnOff   = lipgloss.NewStyle().Foreground(subtle).Padding(0, 2)
)

// Requests from the app; each carries the channel the answer goes back on.
type (
	statusMsg string                      // ui.Info: what's happening now
	logMsg    struct{ kind, text string } // ui.Say / OK / Warn
	chooseReq struct {
		m     ui.Menu
		reply chan chooseRes
	} //
	confirmReq struct {
		q     string
		def   bool
		reply chan bool
	}
	askReq struct {
		q, def   string
		password bool
		reply    chan askRes
	}
	checkReq struct {
		c     ui.Checklist
		reply chan checkRes
	}
	viewReq struct {
		title, text string
		reply       chan struct{}
	}
	chooseRes struct {
		row int
		key string
		err error
	}
	askRes struct {
		s   string
		err error
	}
	checkRes struct {
		on  []bool
		err error
	}
)

type screen int

const (
	busy screen = iota
	choosing
	confirming
	asking
	checking
	viewing
)

type model struct {
	version string
	w, h    int
	scr     screen
	status  string
	spin    spinner.Model
	log     []string // messages since the last question (styled lines)
	logOff  int      // lines scrolled up from the end of the log
	help    *viewport.Model

	// the screen being shown
	menu    chooseReq
	cursor  int
	conf    confirmReq
	yes     bool
	ask     askReq
	input   textinput.Model
	check   checkReq
	on      []bool
	open    []bool
	view    viewReq
	vp      viewport.Model
	quitted bool
}

func newModel(version string) model {
	s := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent)))
	return model{version: version, w: 80, h: 24, spin: s, status: "Starting ..."}
}

func (m model) Init() tea.Cmd { return m.spin.Tick }

// answered: back to the busy screen, and the log starts again.
func (m *model) answered() {
	m.scr, m.log, m.logOff, m.help = busy, nil, 0, nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		if m.help != nil {
			m.help.Width, m.help.Height = m.w, m.h-3
		}
		if m.scr == viewing {
			m.vp.Width, m.vp.Height = m.w, m.h-3
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case statusMsg:
		m.status = string(msg)
		return m, nil
	case logMsg:
		for _, l := range strings.Split(msg.text, "\n") {
			switch msg.kind {
			case "ok":
				l = okStyle.Render(l)
			case "warn":
				l = warnSt.Render(l)
			case "info":
				l = infoSt.Render("==> ") + l
			}
			m.log = append(m.log, l)
		}
		m.logOff = 0
		return m, nil
	case chooseReq:
		m.scr, m.menu, m.cursor = choosing, msg, 0
		return m, nil
	case confirmReq:
		m.scr, m.conf, m.yes = confirming, msg, msg.def
		return m, nil
	case askReq:
		ti := textinput.New()
		ti.Prompt = "> "
		ti.SetValue(msg.def)
		ti.CursorEnd()
		if msg.password {
			ti.EchoMode, ti.EchoCharacter = textinput.EchoPassword, '•'
		}
		ti.Width = max(10, m.w-6)
		cmd := ti.Focus() // before storing it: Focus changes this copy
		m.scr, m.ask, m.input = asking, msg, ti
		return m, cmd
	case checkReq:
		m.scr, m.check, m.cursor = checking, msg, 0
		m.on = make([]bool, len(msg.c.Rows))
		copy(m.on, msg.c.On)
		m.open = make([]bool, len(msg.c.Rows))
		return m, nil
	case viewReq:
		m.scr, m.view = viewing, msg
		m.vp = viewport.New(m.w, m.h-3)
		m.vp.SetContent(msg.text)
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == "ctrl+c" {
		m.quitted = true
		return m, tea.Quit
	}
	if m.help != nil { // help overlay: scroll, any other key closes it
		switch s {
		case "up", "down", "pgup", "pgdown", "home", "end", "k", "j":
			var cmd tea.Cmd
			*m.help, cmd = m.help.Update(k)
			return m, cmd
		}
		m.help = nil
		return m, nil
	}
	switch s { // the log above any question
	case "pgup":
		m.logOff = min(m.logOff+max(1, m.logRows()-1), max(0, len(m.log)-m.logRows()))
		return m, nil
	case "pgdown":
		m.logOff = max(0, m.logOff-max(1, m.logRows()-1))
		return m, nil
	}
	switch m.scr {
	case choosing:
		return m.keyChoose(s)
	case confirming:
		return m.keyConfirm(s)
	case asking:
		return m.keyAsk(k)
	case checking:
		return m.keyCheck(s)
	case viewing:
		switch s {
		case "esc", "enter", "q":
			m.view.reply <- struct{}{}
			m.answered()
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(k)
		return m, cmd
	}
	return m, nil
}

func (m *model) showHelp(text string) {
	if text == "" {
		return
	}
	vp := viewport.New(m.w, m.h-3)
	vp.SetContent(text)
	m.help = &vp
}

func (m model) keyChoose(s string) (tea.Model, tea.Cmd) {
	mn := m.menu.m
	reply := func(r chooseRes) (tea.Model, tea.Cmd) {
		m.menu.reply <- r
		m.answered()
		return m, nil
	}
	switch s {
	case "up", "k":
		m.cursor = (m.cursor - 1 + len(mn.Rows)) % len(mn.Rows)
	case "down", "j", "tab":
		m.cursor = (m.cursor + 1) % len(mn.Rows)
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = len(mn.Rows) - 1
	case "enter":
		return reply(chooseRes{row: m.cursor})
	case "esc":
		if mn.NoBack {
			return reply(chooseRes{err: ui.ErrExit})
		}
		return reply(chooseRes{err: ui.ErrBack})
	case "q", "x":
		if s == "q" || mn.NoBack {
			return reply(chooseRes{err: ui.ErrExit})
		}
	case "?", "h":
		m.showHelp(mn.Help)
	default:
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' && int(s[0]-'1') < len(mn.Rows) {
			m.cursor = int(s[0] - '1')
			return m, nil
		}
		for _, k := range mn.Keys {
			if strings.EqualFold(s, string(k.Letter)) {
				return reply(chooseRes{row: m.cursor, key: k.Name})
			}
		}
	}
	return m, nil
}

func (m model) keyConfirm(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "left", "right", "tab", "shift+tab", "h", "l":
		m.yes = !m.yes
	case "y", "Y":
		m.yes = true
		fallthrough
	case "enter":
		m.conf.reply <- m.yes
		m.answered()
	case "n", "N", "esc":
		m.conf.reply <- false
		m.answered()
	}
	return m, nil
}

func (m model) keyAsk(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "enter":
		v := m.input.Value()
		if v == "" && !m.ask.password {
			v = m.ask.def
		}
		m.ask.reply <- askRes{s: v}
		m.answered()
		return m, nil
	case "esc":
		m.ask.reply <- askRes{err: ui.ErrBack}
		m.answered()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

func (m model) keyCheck(s string) (tea.Model, tea.Cmd) {
	c := m.check.c
	n := len(c.Rows)
	has := func(i int) bool { return i < len(c.Details) && c.Details[i] != "" }
	switch s {
	case "up", "k":
		m.cursor = (m.cursor - 1 + n) % n
	case "down", "j":
		m.cursor = (m.cursor + 1) % n
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = n - 1
	case " ", "x":
		m.on[m.cursor] = !m.on[m.cursor]
	case "right", "l":
		m.open[m.cursor] = has(m.cursor)
	case "left", "h":
		m.open[m.cursor] = false
	case "+":
		for i := range m.open {
			m.open[i] = has(i)
		}
	case "-":
		for i := range m.open {
			m.open[i] = false
		}
	case "a":
		for i := range m.on {
			m.on[i] = true
		}
	case "n":
		for i := range m.on {
			m.on[i] = false
		}
	case "enter":
		m.check.reply <- checkRes{on: append([]bool(nil), m.on...)}
		m.answered()
	case "esc":
		m.check.reply <- checkRes{err: ui.ErrBack}
		m.answered()
	case "q":
		m.check.reply <- checkRes{err: ui.ErrExit}
		m.answered()
	case "?":
		m.showHelp(c.Help)
	}
	return m, nil
}

// ---------------------------------------------------------------- drawing --

func (m model) View() string {
	if m.quitted {
		return ""
	}
	title := "game-mod-sync " + m.version
	if t := m.screenTitle(); t != "" {
		title += "  ·  " + t
	}
	bar := barStyle.Width(m.w).MaxWidth(m.w).Render(title)
	if m.help != nil {
		return bar + "\n" + m.help.View() + "\n" + hints("↑/↓ scroll", "any other key closes the help")
	}
	if m.scr == viewing {
		return bar + "\n" + m.vp.View() + "\n" + hints("↑/↓ PgUp/PgDn scroll", "enter/esc close")
	}
	body, keys := m.widget()
	bodyLines := strings.Split(body, "\n")
	room := m.h - 2 - len(bodyLines) // bar + key line
	var out []string
	out = append(out, bar)
	if room > 0 && len(m.log) > 0 {
		show := m.logLines(room - 1)
		out = append(out, show...)
		out = append(out, dimStyle.Render(strings.Repeat("─", max(0, m.w))))
		room -= len(show) + 1
	}
	out = append(out, bodyLines...)
	for ; room > 0; room-- {
		out = append(out, "")
	}
	out = append(out, keys)
	for i, l := range out {
		out[i] = lipgloss.NewStyle().MaxWidth(m.w).Render(l)
	}
	return strings.Join(out, "\n")
}

func (m model) screenTitle() string {
	switch m.scr {
	case choosing:
		return m.menu.m.Title
	case checking:
		return m.check.c.Title
	case viewing:
		return m.view.title
	}
	return ""
}

// logRows is how many log lines fit above the current question.
func (m model) logRows() int {
	body, _ := m.widget()
	return max(1, m.h-3-strings.Count(body, "\n")-1-1)
}

// logLines is the part of the log that fits in n lines (scrolled by PgUp/PgDn).
func (m model) logLines(n int) []string {
	if n <= 0 {
		return nil
	}
	end := len(m.log) - m.logOff
	start := max(0, end-n)
	lines := append([]string(nil), m.log[start:end]...)
	if start > 0 {
		lines[0] = dimStyle.Render(fmt.Sprintf("↑ %d more line(s) - PgUp", start))
	}
	return lines
}

func hints(pairs ...string) string {
	var parts []string
	for _, p := range pairs {
		k, d, ok := strings.Cut(p, " ")
		if !ok {
			parts = append(parts, keySt.Render(p))
			continue
		}
		parts = append(parts, keySt.Render(k)+" "+dimStyle.Render(d))
	}
	return " " + strings.Join(parts, dimStyle.Render(" • "))
}

// widget is the current question and its key line.
func (m model) widget() (string, string) {
	switch m.scr {
	case choosing:
		return m.viewChoose()
	case confirming:
		yes, no := btnOff.Render("Yes"), btnOff.Render("No")
		if m.yes {
			yes = btnOn.Render("Yes")
		} else {
			no = btnOn.Render("No")
		}
		return boldSt.Render(m.conf.q) + "\n\n  " + yes + "  " + no, hints("←/→ choose", "enter ok", "y yes", "n/esc no")
	case asking:
		k := hints("enter ok", "esc back")
		if m.ask.def != "" && !m.ask.password {
			k = hints("enter ok (empty = "+m.ask.def+")", "esc back")
		}
		return boldSt.Render(m.ask.q) + "\n" + m.input.View(), k
	case checking:
		return m.viewCheck()
	}
	return m.spin.View() + " " + m.status, hints("ctrl+c quit")
}

// window returns the part of n items that fits in rows, keeping cur visible.
func window(n, cur, rows int) (int, int) {
	if rows <= 0 || n <= rows {
		return 0, n
	}
	start := min(max(0, cur-rows/2), n-rows)
	return start, start + rows
}

func (m model) viewChoose() (string, string) {
	mn := m.menu.m
	var b []string
	if mn.Question != "" {
		b = append(b, boldSt.Render(mn.Question), "")
	}
	rows := max(3, m.h-8)
	from, to := window(len(mn.Rows), m.cursor, rows)
	for i := from; i < to; i++ {
		if i == m.cursor {
			b = append(b, curStyle.Render("❯ "+mn.Rows[i]))
		} else {
			b = append(b, "  "+mn.Rows[i])
		}
	}
	keys := []string{"↑/↓ move", "enter ok"}
	for _, k := range mn.Keys {
		keys = append(keys, fmt.Sprintf("%c %s", k.Letter, strings.ToLower(k.Name)))
	}
	if mn.Help != "" {
		keys = append(keys, "? help")
	}
	if mn.NoBack {
		keys = append(keys, "esc/q exit")
	} else {
		keys = append(keys, "esc back")
	}
	return strings.Join(b, "\n"), hints(keys...)
}

func (m model) viewCheck() (string, string) {
	c := m.check.c
	var b []string
	if c.Question != "" {
		for _, l := range strings.Split(c.Question, "\n") {
			b = append(b, boldSt.Render(l))
		}
		b = append(b, "")
	}
	if c.Header != "" {
		b = append(b, dimStyle.Render("       "+c.Header)) // under the rows' text: cursor, arrow, box
	}
	// rendered lines (rows with their open details), and where the cursor is
	type line struct {
		s   string
		row int
	}
	var lines []line
	curLine := 0
	for i, r := range c.Rows {
		box := "[ ]"
		if m.on[i] {
			box = okStyle.Render("[x]")
		}
		arrow := " "
		if i < len(c.Details) && c.Details[i] != "" {
			arrow = dimStyle.Render("▸")
			if m.open[i] {
				arrow = dimStyle.Render("▾")
			}
		}
		text := r
		mark := "  "
		if i == m.cursor {
			text, mark = curStyle.Render(r), curStyle.Render("❯ ")
			curLine = len(lines)
		}
		lines = append(lines, line{mark + arrow + box + " " + text, i})
		if m.open[i] {
			for _, d := range strings.Split(c.Details[i], "\n") {
				lines = append(lines, line{dimStyle.Render("        " + d), i})
			}
		}
	}
	rows := max(3, m.h-6-len(b))
	from, to := window(len(lines), curLine, rows)
	for _, l := range lines[from:to] {
		b = append(b, l.s)
	}
	keys := []string{"↑/↓ move", "space tick", "→/← open/close", "a/n all/none", "enter done", "esc back"}
	if c.Help != "" {
		keys = append(keys, "? help")
	}
	return strings.Join(b, "\n"), hints(keys...)
}
