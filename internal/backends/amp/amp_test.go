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

func TestOptionValue(t *testing.T) {
	line := `-name "{{ServerName}}" -port 2456 -world "{{WorldName}}" -savedir "Saves" {{crossplay}}`
	if got := optionValue(line, "-savedir"); got != "Saves" {
		t.Errorf("got %q", got)
	}
	if got := optionValue(`-savedir "My Saves" -port 1`, "-savedir"); got != "My Saves" {
		t.Errorf("spaces: got %q", got)
	}
	if got := optionValue(`-savedir Plain`, "-savedir"); got != "Plain" {
		t.Errorf("plain: got %q", got)
	}
	if got := optionValue(`-port 1`, "-savedir"); got != "" {
		t.Errorf("missing: got %q", got)
	}
}
