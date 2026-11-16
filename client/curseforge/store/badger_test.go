package store

import (
	"context"
	"slices"
	"testing"

	"github.com/eldius/cli-telemetry-poc/client/curseforge"
)

func testBadger(t *testing.T) *BadgerStore {
	t.Helper()
	db, err := NewBadgerStore(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("NewBadgerStore() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestBadgerSearchMods(t *testing.T) {
	db := testBadger(t)
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
		{"query+version+loader", SearchFilter{Query: "create", GameVersion: "1.20.1", ModLoader: curseforge.ModLoaderForge}, []int{1}},
		{"query+class", SearchFilter{Query: "create", ClassID: curseforge.ClassModpacks}, []int{3}},
		{"limit", SearchFilter{Query: "create", Limit: 1}, []int{1}},
		{"empty filter matches all", SearchFilter{}, []int{4, 1, 2, 3, 6, 5}},
		{"no match", SearchFilter{Query: "no-such-mod"}, []int{}},
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
