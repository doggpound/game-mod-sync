// Package games holds the game files: one <game id>.toml per game (the id is
// the mod manager's folder name for it, e.g. valheim, lethal-company), with
// only the settings that differ from the mod loader's defaults and
// _default.toml (which describes every setting). Adding a game needs no Go
// code: add its file here. They're packed into the program when it's built.
package games

import (
	"embed"

	"github.com/doggpound/game-mod-sync/internal/games"
)

//go:embed *.toml
var files embed.FS

func init() { games.Register(files) }
