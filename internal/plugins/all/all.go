// Package all is the list of plugins built into game-mod-sync: importing it
// registers every one. A new plugin is a folder under internal/plugins/ plus one line
// here.
package all

import (
	// Mod managers: where profiles come from.
	_ "github.com/doggpound/game-mod-sync/internal/plugins/sources/gale"

	// Mod loaders: where mods and configs live.
	_ "github.com/doggpound/game-mod-sync/internal/plugins/loaders/bepinex"

	// Server types: how to find the game folders on a server.
	_ "github.com/doggpound/game-mod-sync/internal/plugins/servers/amp"
	_ "github.com/doggpound/game-mod-sync/internal/plugins/servers/ssh"

	// World formats: how a game saves a world.
	_ "github.com/doggpound/game-mod-sync/internal/plugins/worlds/files"
	_ "github.com/doggpound/game-mod-sync/internal/plugins/worlds/valheim"

	// Game files: settings per game (games/*.toml).
	_ "github.com/doggpound/game-mod-sync/internal/plugins/games"
)
