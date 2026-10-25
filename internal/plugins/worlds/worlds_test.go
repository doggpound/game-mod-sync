package worlds

import "testing"

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{512: "512", 4096: "4.0K", 65 << 20: "65M", 3 << 29: "1.5G"} {
		if got := HumanSize(n); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
}
