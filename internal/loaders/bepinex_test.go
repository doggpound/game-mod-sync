package loaders

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeDLL holds a [BepInPlugin] attribute blob the way the compiler stores it.
func fakeDLL(t *testing.T, path, id, name, version string, extra ...string) {
	t.Helper()
	b := []byte("MZ\x90\x00junk")
	blob := func(ss ...string) {
		b = append(b, 1, 0)
		for _, s := range ss {
			b = append(b, byte(len(s)))
			b = append(b, s...)
		}
		b = append(b, 0, 0)
	}
	blob(id, name, version)
	if len(extra) == 3 {
		blob(extra...)
	}
	b = append(b, "more junk \x01\x00\x05short"...)
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBepInExOwners(t *testing.T) {
	dir := t.TempDir()
	fakeDLL(t, filepath.Join(dir, "Azumatt-FastLink", "FastLink.dll"), "Azumatt.FastLink", "FastLink", "1.4.2")
	fakeDLL(t, filepath.Join(dir, "Argus-QoL", "sub", "QoL.dll"), "argusmagnus.ServersideQoL", "ServersideQoL", "1.0.10")
	// A dependency mentioned as a plain string doesn't count; a bad version doesn't either.
	fakeDLL(t, filepath.Join(dir, "Other-Mod", "Other.dll"), "other.mod", "Other", "1.0", "Azumatt.FastLink", "x", "not-a-version")
	fakeDLL(t, filepath.Join(dir, "MMHOOK", "MMHOOK_x.dll"), "hook.id", "Hook", "1.0")
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("\x01\x00\x03abc\x03def\x051.2.3"), 0o644)
	want := map[string]string{"Azumatt.FastLink": "Azumatt-FastLink", "argusmagnus.ServersideQoL": "Argus-QoL", "other.mod": "Other-Mod"}
	if got := BepInExOwners(dir); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}
