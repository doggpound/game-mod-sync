// Package ssh is the "Other" server type: any server reached with SSH or SFTP
// details, like the ones a game host gives out. Game folders are searched
// from where the login starts; the folder can always be typed instead.
package ssh

import (
	"github.com/doggpound/game-mod-sync/internal/plugin"
	"github.com/doggpound/game-mod-sync/internal/state"
)

func init() { plugin.RegisterServer(Server{}) }

// Server is the "Other" server type.
type Server struct{}

// Name is "ssh" (what 0.x settings call it); never change it.
func (Server) Name() string  { return "ssh" }
func (Server) Label() string { return "Other: SSH or SFTP details from your host" }
func (Server) Help() string {
	return "any server you can log in to with SSH or SFTP: the address, port, user name and password your host gave you"
}
func (Server) DefaultUser() string { return "" }
func (Server) Fallback() bool      { return true }

func (Server) SearchFrom(_ *state.Config, home string, _ bool) (string, int) { return home, 6 }

func (Server) ShortName(_ *state.Config, root string) string { return root }
