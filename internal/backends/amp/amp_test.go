package amp

import "testing"

func TestInstance(t *testing.T) {
	dir, name, ok := Instance("/home/amp/.ampdata/instances/Valheim01/Valheim/896660", ".ampdata/instances")
	if !ok || name != "Valheim01" || dir != "/home/amp/.ampdata/instances/Valheim01" {
		t.Errorf("%q %q %v", dir, name, ok)
	}
	if _, _, ok := Instance("/srv/valheim", ".ampdata/instances"); ok {
		t.Error("a folder outside AMP counted as an instance")
	}
	if got := ShortName("/srv/valheim", ".ampdata/instances"); got != "/srv/valheim" {
		t.Errorf("ShortName = %q", got)
	}
}
