package all

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every plugin folder must be in all.go, or the program never sees it.
func TestEveryPluginIsListed(t *testing.T) {
	list, err := os.ReadFile("all.go")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir("..", func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == ".." || filepath.Base(p) == "all" {
			return err
		}
		// A plugin registers itself in init; a folder without one (shared
		// code such as worlds/) isn't a plugin.
		goFiles, _ := filepath.Glob(filepath.Join(p, "*.go"))
		registers := false
		for _, f := range goFiles {
			b, _ := os.ReadFile(f)
			if !strings.HasSuffix(f, "_test.go") && strings.Contains(string(b), "\nfunc init()") {
				registers = true
			}
		}
		if !registers {
			return nil
		}
		imp := "github.com/doggpound/game-mod-sync/internal/plugins/" + filepath.ToSlash(strings.TrimPrefix(p, ".."+string(filepath.Separator)))
		if !strings.Contains(string(list), `"`+imp+`"`) {
			t.Errorf("%s isn't in internal/plugins/all/all.go - add: _ %q", p, imp)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
