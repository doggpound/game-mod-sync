// Package games holds the game files and builds each game's settings from
// them: what to sync, what to leave alone, how to recognise it on a server.
//
// The game files are one <game id>.toml per game in this folder (the id is the
// mod manager's folder name for it, e.g. valheim, lethal-company), with only
// the settings that differ from the mod loader's defaults, and _default.toml
// (which describes every setting). Adding a game needs no Go code: add its
// file here. They're packed into the program when it's built. They're read in
// order - _default.toml, then <game id>.toml - after the mod loader's
// defaults, and a file with the same name in the user's games folder changes
// them again (only what it lists).
package games

import "embed"

//go:embed *.toml
var files embed.FS

func init() { Register(files) }
