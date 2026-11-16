package curseforge

import "testing"

func TestResolveLoader(t *testing.T) {
	cases := map[string]int{
		"":           ModLoaderAny,
		"any":        ModLoaderAny,
		"forge":      ModLoaderForge,
		"Fabric":     ModLoaderFabric,
		"NEOFORGE":   ModLoaderNeoForge,
		" quilt ":    ModLoaderQuilt,
		"liteloader": ModLoaderLiteLoader,
		"cauldron":   ModLoaderCauldron,
		"4":          ModLoaderFabric,
	}
	for name, want := range cases {
		got, err := ResolveLoader(name)
		if err != nil {
			t.Errorf("ResolveLoader(%q) error: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("ResolveLoader(%q) = %d, want %d", name, got, want)
		}
	}
	if _, err := ResolveLoader("unknown-loader"); err == nil {
		t.Error("ResolveLoader(unknown-loader) should return an error")
	}
}

func TestLoaderName(t *testing.T) {
	if got := LoaderName(ModLoaderNeoForge); got != "neoforge" {
		t.Errorf("LoaderName(ModLoaderNeoForge) = %q, want neoforge", got)
	}
	if got := LoaderName(999); got != "loader-999" {
		t.Errorf("LoaderName(999) = %q, want loader-999", got)
	}
}
