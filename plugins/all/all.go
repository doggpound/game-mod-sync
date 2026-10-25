// Package all is the list of plugins built into game-mod-sync: importing it
// registers every one. A new plugin is a folder under plugins/ plus one line
// here.
package all

import (
	// Mod managers: where profiles come from.
	_ "github.com/doggpound/game-mod-sync/plugins/sources/gale"

	// Mod loaders: where mods and configs live.
	_ "github.com/doggpound/game-mod-sync/plugins/loaders/bepinex"

	// Server types: how to find the game folders on a server.
	_ "github.com/doggpound/game-mod-sync/plugins/servers/amp"
	_ "github.com/doggpound/game-mod-sync/plugins/servers/ssh"

	// World formats: how a game saves a world.
	_ "github.com/doggpound/game-mod-sync/plugins/worlds/files"
	_ "github.com/doggpound/game-mod-sync/plugins/worlds/valheim"

	// Game files: settings per game (plugins/games/*.toml).
	_ "github.com/doggpound/game-mod-sync/plugins/games"
)
