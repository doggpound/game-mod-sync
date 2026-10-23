// Package text is the text interface: numbered menus and plain prompts. It
// also works without a terminal (answers from a pipe, or -y).
package text

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/term"

	"github.com/doggpound/game-mod-sync/internal/ui"
)

// Text implements ui.UI on a reader and a writer.
type Text struct {
	in    *bufio.Reader
	out   io.Writer
	color bool
	// password reads a hidden line; nil = read a plain line from in.
	password func() (string, error)
}

// New is the text interface on in and out. When in is the terminal,
// passwords aren't shown; when out is a terminal, colours are used.
func New(in io.Reader, out io.Writer) *Text {
	t := &Text{in: bufio.NewReader(in), out: out}
	if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) && os.Getenv("NO_COLOR") == "" {
		t.color = true
	}
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fd := int(f.Fd())
		t.password = func() (string, error) {
			b, err := term.ReadPassword(fd)
			fmt.Fprintln(t.out)
			return string(b), err
		}
	}
	return t
}

func (t *Text) paint(code, s string) string {
	if !t.color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Bold, Dim are for labels the app builds.
func (t *Text) Bold(s string) string { return t.paint("1", s) }
func (t *Text) Dim(s string) string  { return t.paint("2", s) }

func (t *Text) Info(msg string) { fmt.Fprintln(t.out, t.paint("1;34", "==>")+" "+msg) }
func (t *Text) Say(msg string)  { fmt.Fprintln(t.out, msg) }
func (t *Text) OK(msg string)   { fmt.Fprintln(t.out, t.paint("32", msg)) }
func (t *Text) Warn(msg string) { fmt.Fprintln(t.out, t.paint("33", "warning:")+" "+msg) }

func (t *Text) line() (string, error) {
	s, err := t.in.ReadString('\n')
	if err != nil && s == "" {
		return "", fmt.Errorf("no more input: %w", ui.ErrExit)
	}
	return strings.TrimSpace(s), nil
}

func (t *Text) Choose(m ui.Menu) (int, string, error) {
	if m.Title != "" {
		fmt.Fprintf(t.out, "\n%s\n", t.Bold("-- "+m.Title+" --"))
	}
	list := func() {
		for i, r := range m.Rows {
			fmt.Fprintf(t.out, "  %s %s\n", t.Bold(fmt.Sprintf("%2d)", i+1)), r)
		}
	}
	list()
	keys := map[rune]string{}
	var hints []string
	for _, k := range m.Keys {
		keys[unicode.ToLower(k.Letter)] = k.Name
		hints = append(hints, fmt.Sprintf("<number>%c = %s", k.Letter, strings.ToLower(k.Name)))
	}
	if m.Help != "" {
		hints = append(hints, "h = help")
	}
	if m.NoBack {
		hints = append(hints, "x = exit")
	} else {
		hints = append(hints, "b = back")
	}
	hints = append(hints, "q to quit")
	for {
		fmt.Fprintf(t.out, "%s [1-%d, %s]: ", m.Question, len(m.Rows), strings.Join(hints, ", "))
		a, err := t.line()
		if err != nil {
			return 0, "", err
		}
		switch strings.ToLower(a) {
		case "q":
			return 0, "", ui.ErrExit
		case "x":
			if m.NoBack {
				return 0, "", ui.ErrExit
			}
		case "b":
			if !m.NoBack {
				return 0, "", ui.ErrBack
			}
		case "h":
			if m.Help != "" {
				fmt.Fprintf(t.out, "\n%s\n\n", m.Help)
				list()
				continue
			}
		}
		num, key := a, ""
		if n := len(a); n > 1 && unicode.IsLetter(rune(a[n-1])) {
			if name, ok := keys[unicode.ToLower(rune(a[n-1]))]; ok {
				num, key = a[:n-1], name
			}
		}
		if i, err := strconv.Atoi(num); err == nil && i >= 1 && i <= len(m.Rows) {
			return i - 1, key, nil
		}
		fmt.Fprintf(t.out, "%s pick a number between 1 and %d\n", t.paint("33", "warning:"), len(m.Rows))
	}
}

func (t *Text) Confirm(q string, defYes bool) bool {
	hint := "[y/N]"
	if defYes {
		hint = "[Y/n]"
	}
	fmt.Fprintf(t.out, "%s %s ", q, hint)
	a, err := t.line()
	if err != nil {
		return defYes // no answer (input ended): the default, which is the safe choice
	}
	switch strings.ToLower(a) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	}
	return defYes && a == ""
}

func (t *Text) Ask(q, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(t.out, "%s [%s]: ", q, def)
	} else {
		fmt.Fprintf(t.out, "%s: ", q)
	}
	a, err := t.line()
	if err != nil {
		return "", err
	}
	if a == "b" {
		return "", ui.ErrBack
	}
	if a == "" {
		return def, nil
	}
	return a, nil
}

func (t *Text) Password(q string) (string, error) {
	fmt.Fprintf(t.out, "%s: ", q)
	if t.password != nil {
		return t.password()
	}
	return t.line()
}

func (t *Text) View(title, text string) {
	if title != "" {
		fmt.Fprintf(t.out, "\n%s\n", t.Bold("-- "+title+" --"))
	}
	fmt.Fprintln(t.out, text)
}

// Check is a numbered list with [x] marks: numbers toggle rows, a / n tick
// all or none, d shows the details, Enter is done, b goes back.
func (t *Text) Check(c ui.Checklist) ([]bool, error) {
	on := append([]bool(nil), c.On...)
	for len(on) < len(c.Rows) {
		on = append(on, false)
	}
	hasDetails := false
	for _, d := range c.Details {
		if d != "" {
			hasDetails = true
		}
	}
	list := func() {
		if c.Title != "" {
			fmt.Fprintf(t.out, "\n%s\n", t.Bold("-- "+c.Title+" --"))
		}
		if c.Question != "" {
			fmt.Fprintln(t.out, c.Question)
		}
		if c.Header != "" {
			fmt.Fprintf(t.out, "          %s\n", t.Dim(c.Header))
		}
		for i, r := range c.Rows {
			mark := "[ ]"
			if on[i] {
				mark = t.paint("32", "[x]")
			}
			fmt.Fprintf(t.out, "  %s %s %s\n", t.Bold(fmt.Sprintf("%2d)", i+1)), mark, r)
		}
	}
	list()
	hints := "numbers to tick or untick (e.g. 1 3), a = all, n = none"
	if hasDetails {
		hints += ", d = show the differences"
	}
	if c.Help != "" {
		hints += ", h = help"
	}
	hints += ", Enter = done, b = back"
	for {
		fmt.Fprintf(t.out, "[%s]: ", hints)
		a, err := t.line()
		if err != nil {
			return on, nil // input ended: the ticks as they are (the defaults)
		}
		switch strings.ToLower(a) {
		case "":
			return on, nil
		case "b":
			return nil, ui.ErrBack
		case "q":
			return nil, ui.ErrExit
		case "a", "n":
			for i := range on {
				on[i] = strings.ToLower(a) == "a"
			}
			list()
			continue
		case "d":
			if hasDetails {
				for i, d := range c.Details {
					if d != "" {
						fmt.Fprintf(t.out, "\n%s\n%s\n", c.Rows[i], d)
					}
				}
				fmt.Fprintln(t.out)
				list()
				continue
			}
		case "h":
			if c.Help != "" {
				fmt.Fprintf(t.out, "\n%s\n", c.Help)
				list()
				continue
			}
		}
		ok := true
		for _, f := range strings.FieldsFunc(a, func(r rune) bool { return r == ' ' || r == ',' }) {
			i, err := strconv.Atoi(f)
			if err != nil || i < 1 || i > len(on) {
				ok = false
				break
			}
			on[i-1] = !on[i-1]
		}
		if !ok {
			fmt.Fprintf(t.out, "%s type row numbers between 1 and %d\n", t.paint("33", "warning:"), len(on))
		}
		list()
	}
}
