package lease

import "testing"

func TestNormalize(t *testing.T) {
	for _, in := range []string{"cpu-q", "resource:cpu-q", " resource:cpu-q "} {
		if got := Normalize(in); got.Key != "cpu-q" {
			t.Errorf("Normalize(%q).Key = %q", in, got.Key)
		}
	}
	if got := Normalize("cpu-q").ID(); got != "resource:cpu-q" {
		t.Errorf("ID() = %q", got)
	}
	if got := Normalize("cpu-q").Title(); got != "Resource lease: cpu-q" {
		t.Errorf("Title() = %q", got)
	}
}
