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
			m.help.Width, m.help.Height = m.w-4, m.h-5
		}
		if m.scr == viewing {
			m.vp.Width, m.vp.Height = m.w-4, m.h-5
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
		m.vp = viewport.New(m.w-4, m.h-5)
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
		m.logOff = min(m.logOff+max(1, m.logRows()-1), max(0, len(m.shownLog())-m.logRows()))
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
	vp := viewport.New(m.w-4, m.h-5)
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
//
// Layout, top to bottom, two columns in from the left:
//
//	 game-mod-sync  1.0.0   Main menu          title badge, version, screen
//
//	╭──────────────────────────────╮            messages since the last
//	│ Synced.                      │            question (PgUp/PgDn), if any
//	╰──────────────────────────────╯
//	Which profile do you want to sync?          the question
//	╭──────────────────────────────╮
//	│ ❯ Valheim / MyProfile  → ... │            the list, in a box
//	╰──────────────────────────────╯
//	↑/↓ move • enter ok • ...                   its keys, right under it

const margin = "  "

var (
	listBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
	logBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle).Padding(0, 1)
)

func (m model) View() string {
	if m.quitted {
		return ""
	}
	head := barStyle.Render("game-mod-sync") + " " + dimStyle.Render(m.version)
	if t := m.screenTitle(); t != "" {
		head += "   " + boldSt.Render(t)
	}
	out := []string{"", head, ""}
	switch {
	case m.help != nil:
		out = append(out, m.help.View(), "", hints("↑/↓ scroll", "any other key closes the help"))
	case m.scr == viewing:
		out = append(out, m.vp.View(), "", hints("↑/↓ PgUp/PgDn scroll", "enter/esc close"))
	default:
		body := m.widget()
		if n := m.logRoom(len(body)); n > 0 {
			if lines := m.logLines(n); len(lines) > 0 {
				out = append(out, strings.Split(m.boxed(logBox, lines), "\n")...)
			}
		}
		out = append(out, body...)
	}
	for i, l := range out {
		out[i] = lipgloss.NewStyle().MaxWidth(m.w).Render(margin + l)
	}
	if len(out) > m.h {
		out = out[:m.h]
	}
	return strings.Join(out, "\n")
}

// inner is the widest a box's contents can be.
func (m model) inner() int { return max(20, m.w-len(margin)*2-4) }

// boxed draws lines in a box as wide as its widest line (up to the screen).
func (m model) boxed(st lipgloss.Style, lines []string) string {
	w := 0
	for _, l := range lines {
		w = max(w, lipgloss.Width(l))
	}
	w = min(max(w, 30), m.inner())
	for i, l := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(w).Render(l)
	}
	return st.Width(w + 2).Render(strings.Join(lines, "\n"))
}

// logRoom is how many message lines fit above a question of n lines.
func (m model) logRoom(n int) int {
	return m.h - 3 - n - 2 // header, the question, the log box's borders
}

// logRows is how many log lines are shown now (PgUp/PgDn move by that).
func (m model) logRows() int { return max(1, m.logRoom(len(m.widget()))) }

// shownLog is the log without the blank lines at its start and end (the app
// prints those as spacing for the text interface).
func (m model) shownLog() []string {
	l := m.log
	for len(l) > 0 && strings.TrimSpace(l[0]) == "" {
		l = l[1:]
	}
	for len(l) > 0 && strings.TrimSpace(l[len(l)-1]) == "" {
		l = l[:len(l)-1]
	}
	return l
}

// logLines is the part of the log that fits in n lines (scrolled by PgUp/PgDn).
func (m model) logLines(n int) []string {
	log := m.shownLog()
	end := max(0, len(log)-m.logOff)
	start := max(0, end-n)
	lines := append([]string(nil), log[start:end]...)
	if start > 0 {
		lines[0] = dimStyle.Render(fmt.Sprintf("↑ %d more line(s) - PgUp", start))
	}
	return lines
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
	return strings.Join(parts, dimStyle.Render(" • "))
}

// widget is the current question, as lines.
func (m model) widget() []string {
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
		return []string{boldSt.Render(m.conf.q), "", yes + "  " + no, "", hints("←/→ choose", "enter ok", "y yes", "n/esc no")}
	case asking:
		k := hints("enter ok", "esc back")
		if m.ask.def != "" && !m.ask.password {
			k = hints("enter ok (empty = "+m.ask.def+")", "esc back")
		}
		box := strings.Split(listBox.Width(min(m.inner(), 60)+2).Render(m.input.View()), "\n")
		return append(append([]string{boldSt.Render(m.ask.q)}, box...), k)
	case checking:
		return m.viewCheck()
	}
	return []string{m.spin.View() + " " + m.status, "", hints("ctrl+c quit")}
}

// window returns the part of n items that fits in rows, keeping cur visible.
func window(n, cur, rows int) (int, int) {
	if rows <= 0 || n <= rows {
		return 0, n
	}
	start := min(max(0, cur-rows/2), n-rows)
	return start, start + rows
}

// columns lines up rows whose parts are separated by tabs: the first part
// as is, the others dimmed (they're details: where a profile syncs to).
func columns(rows []string) [][2]string {
	var widths []int
	split := make([][]string, len(rows))
	for i, r := range rows {
		split[i] = strings.Split(r, "\t")
		for j, p := range split[i] {
			if j >= len(widths) {
				widths = append(widths, 0)
			}
			widths[j] = max(widths[j], lipgloss.Width(p))
		}
	}
	out := make([][2]string, len(rows))
	for i, parts := range split {
		first := parts[0]
		if len(parts) > 1 {
			first += strings.Repeat(" ", widths[0]-lipgloss.Width(parts[0]))
		}
		var rest []string
		for j, p := range parts[1:] {
			if j < len(parts)-2 {
				p += strings.Repeat(" ", widths[j+1]-lipgloss.Width(p))
			}
			rest = append(rest, p)
		}
		out[i] = [2]string{first, strings.Join(rest, "  ")}
	}
	return out
}

func (m model) viewChoose() []string {
	mn := m.menu.m
	var b []string
	if mn.Question != "" {
		b = append(b, boldSt.Render(mn.Question))
	}
	rows := max(3, m.h-3-len(b)-3)
	from, to := window(len(mn.Rows), m.cursor, rows)
	cols := columns(mn.Rows)
	var lines []string
	if from > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ↑ %d more", from)))
	}
	for i := from; i < to; i++ {
		first, rest := cols[i][0], cols[i][1]
		if rest != "" {
			rest = "  " + rest
		}
		if i == m.cursor {
			lines = append(lines, curStyle.Render("❯ "+first)+curStyle.Render(rest))
		} else {
			lines = append(lines, "  "+first+dimStyle.Render(rest))
		}
	}
	if to < len(mn.Rows) {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ↓ %d more", len(mn.Rows)-to)))
	}
	b = append(b, strings.Split(m.boxed(listBox, lines), "\n")...)
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
	return append(b, hints(keys...))
}

func (m model) viewCheck() []string {
	c := m.check.c
	var b []string
	for i, l := range strings.Split(c.Question, "\n") {
		if l == "" {
			continue
		}
		if i == 0 {
			b = append(b, boldSt.Render(l))
		} else {
			b = append(b, warnSt.Render(l)) // notes: client-only mods, hidden files
		}
	}
	// rendered lines (rows with their open details), and the cursor's line
	var lines []string
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
		mark, text := "  ", r
		if i == m.cursor {
			mark, text = curStyle.Render("❯ "), curStyle.Render(r)
			curLine = len(lines)
		}
		lines = append(lines, mark+box+" "+arrow+" "+text)
		if m.open[i] {
			for _, d := range strings.Split(c.Details[i], "\n") {
				lines = append(lines, dimStyle.Render("        "+d))
			}
		}
	}
	rows := max(3, m.h-3-len(b)-4)
	from, to := window(len(lines), curLine, rows)
	var shown []string
	if c.Header != "" {
		shown = append(shown, dimStyle.Render("        "+c.Header)) // under the rows' text: cursor, box, arrow
	}
	if from > 0 {
		shown = append(shown, dimStyle.Render(fmt.Sprintf("  ↑ %d more", from)))
	}
	shown = append(shown, lines[from:to]...)
	if to < len(lines) {
		shown = append(shown, dimStyle.Render(fmt.Sprintf("  ↓ %d more", len(lines)-to)))
	}
	b = append(b, strings.Split(m.boxed(listBox, shown), "\n")...)
	keys := []string{"↑/↓ move", "space tick", "→/← open/close", "a/n all/none", "enter done", "esc back"}
	if c.Help != "" {
		keys = append(keys, "? help")
	}
	return append(b, hints(keys...))
}
