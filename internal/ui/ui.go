// Package ui is how game-mod-sync talks to the person using it. The app never
// prints or reads directly; it calls a UI, so the same flow works in the text
// interface (ui/text) and the full-screen one (phase 4).
package ui

import "errors"

// ErrBack is returned when the person goes back (Back button, b).
var ErrBack = errors.New("back")

// ErrExit is returned when the person exits (Exit button, x or q).
var ErrExit = errors.New("exit")

// Key is a row key in a menu: pressed on a row, it returns Name for that row
// (e.g. 'e' Edit, 'd' Download).
type Key struct {
	Letter rune
	Name   string
}

// Menu is a list to pick one row from.
type Menu struct {
	Title    string // heading, e.g. "Main menu"
	Question string
	Rows     []string
	Keys     []Key  // row keys
	Help     string // shown on h / H
	NoBack   bool   // main menu: Exit instead of Back
}

// Checklist is a list of rows to tick.
type Checklist struct {
	Title    string
	Question string // shown above the list (may be several lines)
	Header   string // column titles, lined up with the rows' text
	Rows     []string
	On       []bool   // ticked at the start
	Details  []string // per row, shown when it's opened (full screen) or on d (text); may be nil
	Help     string
}

// UI is everything the app asks of an interface.
type UI interface {
	Info(msg string) // something is happening: "Comparing ..."
	Say(msg string)  // plain text
	OK(msg string)   // a result
	Warn(msg string) // a warning

	// Choose returns the picked row (0-based) and, if a row key was used,
	// its Name. ErrBack and ErrExit when the person leaves.
	Choose(m Menu) (row int, key string, err error)
	// Confirm asks a yes/no question; yes is the default when defYes.
	Confirm(question string, defYes bool) bool
	// Ask asks for a line of text; def is offered as the answer. ErrBack on b.
	Ask(question, def string) (string, error)
	// Password asks without showing what's typed.
	Password(question string) (string, error)
	// Check shows a checklist and returns which rows are ticked. ErrBack on Back.
	Check(c Checklist) ([]bool, error)
	// View shows a long text (full screen: a scrollable box).
	View(title, text string)
}
