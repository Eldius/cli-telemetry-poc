package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/eldius/cli-telemetry-poc/client/curseforge"
)

func testDB(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func testMod(id int, name string) curseforge.ModEnvelope {
	raw, _ := json.Marshal(map[string]any{"id": id, "name": name})
	return curseforge.ModEnvelope{
		Mod: curseforge.Mod{
			ID:            id,
			GameID:        curseforge.GameIDMinecraft,
			ClassID:       curseforge.ClassMods,
			Name:          name,
			Slug:          name,
			DownloadCount: 100,
			Authors:       []curseforge.Author{{ID: 1, Name: "author"}},
			Categories:    []curseforge.Category{{ID: 417, Name: "Tech"}},
			DateModified:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Raw: raw,
	}
}

func TestUpsertModsInsertAndUpdate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if err := db.UpsertMods(ctx, []curseforge.ModEnvelope{testMod(1, "mod-a"), testMod(2, "mod-b")}); err != nil {
		t.Fatalf("UpsertMods() error: %v", err)
	}

	updated := testMod(1, "mod-a")
	updated.Mod.DownloadCount = 999
	if err := db.UpsertMods(ctx, []curseforge.ModEnvelope{updated}); err != nil {
		t.Fatalf("UpsertMods() update error: %v", err)
	}

	var count int
	if err := db.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM mods`); err != nil {
		t.Fatalf("counting mods: %v", err)
	}
	if count != 2 {
		t.Errorf("got %d mods, want 2 (upsert must not duplicate)", count)
	}

	var downloads int64
	if err := db.db.GetContext(ctx, &downloads, `SELECT download_count FROM mods WHERE id = 1`); err != nil {
		t.Fatalf("reading mod: %v", err)
	}
	if downloads != 999 {
		t.Errorf("download_count = %d, want 999", downloads)
	}

	var raw string
	if err := db.db.GetContext(ctx, &raw, `SELECT raw_json FROM mods WHERE id = 1`); err != nil {
		t.Fatalf("reading raw json: %v", err)
	}
	if !json.Valid([]byte(raw)) {
		t.Errorf("raw_json is not valid JSON: %s", raw)
	}
}

func TestUpsertCategories(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cats := []curseforge.Category{
		{ID: 6, GameID: curseforge.GameIDMinecraft, Name: "Mods", Slug: "mc-mods", IsClass: true},
		{ID: 417, GameID: curseforge.GameIDMinecraft, ClassID: 6, Name: "Tech", Slug: "tech"},
	}
	if err := db.UpsertCategories(ctx, cats); err != nil {
		t.Fatalf("UpsertCategories() error: %v", err)
	}
	// Re-upsert must not fail nor duplicate.
	if err := db.UpsertCategories(ctx, cats); err != nil {
		t.Fatalf("UpsertCategories() second call error: %v", err)
	}

	var count int
	if err := db.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM categories`); err != nil {
		t.Fatalf("counting categories: %v", err)
	}
	if count != 2 {
		t.Errorf("got %d categories, want 2", count)
	}
}

func TestSyncStateRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if _, ok, err := db.GetSyncState(ctx, "missing"); err != nil || ok {
		t.Errorf("GetSyncState(missing) = ok=%v, err=%v; want ok=false, err=nil", ok, err)
	}

	if err := db.SetSyncState(ctx, "k", "v1"); err != nil {
		t.Fatalf("SetSyncState() error: %v", err)
	}
	if err := db.SetSyncState(ctx, "k", "v2"); err != nil {
		t.Fatalf("SetSyncState() update error: %v", err)
	}

	got, ok, err := db.GetSyncState(ctx, "k")
	if err != nil || !ok || got != "v2" {
		t.Errorf("GetSyncState(k) = %q, %v, %v; want v2, true, nil", got, ok, err)
	}
}

func TestStats(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	mods := []curseforge.ModEnvelope{testMod(1, "mod-a"), testMod(2, "mod-b")}
	if err := db.UpsertMods(ctx, mods); err != nil {
		t.Fatalf("UpsertMods() error: %v", err)
	}
	modpack := testMod(3, "pack-a")
	modpack.Mod.ClassID = curseforge.ClassModpacks
	if err := db.UpsertMods(ctx, []curseforge.ModEnvelope{modpack}); err != nil {
		t.Fatalf("UpsertMods() error: %v", err)
	}
	if err := db.SetSyncState(ctx, "sync.class.6.last_update", "2024-01-01T00:00:00Z"); err != nil {
		t.Fatalf("SetSyncState() error: %v", err)
	}

	stats, err := db.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}
	if stats.TotalMods != 3 {
		t.Errorf("TotalMods = %d, want 3", stats.TotalMods)
	}
	if len(stats.Classes) != 2 {
		t.Errorf("got %d class stats, want 2", len(stats.Classes))
	}
	if got := stats.SyncWatermarks["sync.class.6.last_update"]; got != "2024-01-01T00:00:00Z" {
		t.Errorf("watermark = %q", got)
	}
}

// testModSearch builds a mod with a lossless raw payload, as stored by sync.
func testModSearch(id, classID int, name string, downloads int64, indexes []curseforge.FileIndex) curseforge.ModEnvelope {
	m := curseforge.Mod{
		ID:                 id,
		GameID:             curseforge.GameIDMinecraft,
		ClassID:            classID,
		Name:               name,
		Slug:               name,
		Summary:            "summary of " + name,
		DownloadCount:      downloads,
		LatestFilesIndexes: indexes,
		DateModified:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	raw, _ := json.Marshal(m)
	return curseforge.ModEnvelope{Mod: m, Raw: raw}
}

// searchFixtures is shared by the SQLite and BadgerDB search tests.
func searchFixtures() []curseforge.ModEnvelope {
	return []curseforge.ModEnvelope{
		testModSearch(1, curseforge.ClassMods, "Create", 5000, []curseforge.FileIndex{
			{GameVersion: "1.20.1", ModLoader: curseforge.ModLoaderForge},
			{GameVersion: "1.20.1", ModLoader: curseforge.ModLoaderFabric},
		}),
		testModSearch(2, curseforge.ClassMods, "Create Crafts", 3000, []curseforge.FileIndex{
			{GameVersion: "1.19.2", ModLoader: curseforge.ModLoaderForge},
		}),
		testModSearch(3, curseforge.ClassModpacks, "Create Pack", 1000, []curseforge.FileIndex{
			{GameVersion: "1.20.1", ModLoader: curseforge.ModLoaderNeoForge},
		}),
		testModSearch(4, curseforge.ClassMods, "JEI", 9000, []curseforge.FileIndex{
			{GameVersion: "1.20.1", ModLoader: curseforge.ModLoaderForge},
		}),
		testModSearch(5, curseforge.ClassMods, "wild_card", 10, nil),
		testModSearch(6, curseforge.ClassMods, "wildXcard", 20, nil),
	}
}

func searchIDs(t *testing.T, mods []curseforge.ModEnvelope) []int {
	t.Helper()
	ids := make([]int, 0, len(mods))
	for _, env := range mods {
		ids = append(ids, env.Mod.ID)
	}
	return ids
}

func TestSearchMods(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if err := db.UpsertMods(ctx, searchFixtures()); err != nil {
		t.Fatalf("UpsertMods() error: %v", err)
	}

	tests := []struct {
		name   string
		filter SearchFilter
		want   []int
	}{
		{"query is case-insensitive, ordered by downloads", SearchFilter{Query: "create"}, []int{1, 2, 3}},
		{"uppercase query", SearchFilter{Query: "CREATE"}, []int{1, 2, 3}},
		{"query+version+loader", SearchFilter{Query: "create", GameVersion: "1.20.1", ModLoader: curseforge.ModLoaderForge}, []int{1}},
		{"query+class", SearchFilter{Query: "create", ClassID: curseforge.ClassModpacks}, []int{3}},
		{"version only", SearchFilter{GameVersion: "1.20.1"}, []int{4, 1, 3}},
		{"loader only", SearchFilter{ModLoader: curseforge.ModLoaderFabric}, []int{1}},
		{"limit", SearchFilter{Query: "create", Limit: 1}, []int{1}},
		{"like wildcards are escaped", SearchFilter{Query: "wild_card"}, []int{5}},
		{"empty filter matches all", SearchFilter{}, []int{4, 1, 2, 3, 6, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.SearchMods(ctx, tt.filter)
			if err != nil {
				t.Fatalf("SearchMods(%+v) error: %v", tt.filter, err)
			}
			if ids := searchIDs(t, got); !slices.Equal(ids, tt.want) {
				t.Errorf("SearchMods(%+v) = %v, want %v", tt.filter, ids, tt.want)
			}
		})
	}
}
