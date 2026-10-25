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
		goFiles, _ := filepath.Glob(filepath.Join(p, "*.go"))
		hasCode := false
		for _, f := range goFiles {
			if !strings.HasSuffix(f, "_test.go") {
				hasCode = true
			}
		}
		if !hasCode {
			return nil
		}
		imp := "github.com/doggpound/game-mod-sync/plugins/" + filepath.ToSlash(strings.TrimPrefix(p, ".."+string(filepath.Separator)))
		if !strings.Contains(string(list), `"`+imp+`"`) {
			t.Errorf("%s isn't in plugins/all/all.go - add: _ %q", p, imp)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
