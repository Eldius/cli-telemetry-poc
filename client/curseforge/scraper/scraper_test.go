package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	curseforge2 "github.com/eldius/cli-telemetry-poc/client/curseforge"
	store2 "github.com/eldius/cli-telemetry-poc/client/curseforge/store"
)

// fakeAPI implements a small in-memory CurseForge API for tests.
type fakeAPI struct {
	mu sync.Mutex
	// modsByVersion maps game version -> mod JSON payloads.
	modsByVersion map[string][]string
	// recentMods are returned for LastUpdated-sorted searches (incremental).
	recentMods []string
	// totalOverrides maps "classId|version" to a fake totalCount.
	totalOverrides map[string]int
	// requestedVersions records the gameVersion params of search requests.
	requestedVersions []string
}

func modJSON(id int, classID int, dateModified time.Time) string {
	return fmt.Sprintf(`{
		"id": %d, "gameId": 432, "classId": %d, "name": "mod-%d", "slug": "mod-%d",
		"links": {"websiteUrl": "https://example.com/mod-%d"},
		"summary": "test mod %d", "status": 4, "downloadCount": %d,
		"authors": [{"id": 1, "name": "tester"}],
		"categories": [{"id": 417, "gameId": 432, "name": "Tech", "classId": 6}],
		"latestFiles": [], "latestFilesIndexes": [],
		"dateCreated": "2020-01-01T00:00:00Z",
		"dateModified": "%s",
		"dateReleased": "2020-01-01T00:00:00Z",
		"isAvailable": true
	}`, id, classID, id, id, id, id, id*100, dateModified.UTC().Format(time.RFC3339))
}

func (f *fakeAPI) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/categories":
			_, _ = fmt.Fprint(w, `{"data": [{"id": 417, "gameId": 432, "name": "Tech", "slug": "tech", "classId": 6, "isClass": false}]}`)
		case "/v1/games/432/versions":
			_, _ = fmt.Fprint(w, `{"data": [
				{"id": 1, "gameId": 432, "name": "1.20.1", "gameVersionTypeId": 100},
				{"id": 2, "gameId": 432, "name": "1.19.2", "gameVersionTypeId": 100},
				{"id": 3, "gameId": 432, "name": "23w13a", "gameVersionTypeId": 100}
			]}`)
		case "/v1/mods/search":
			f.serveSearch(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (f *fakeAPI) serveSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	classID, _ := strconv.Atoi(q.Get("classId"))
	version := q.Get("gameVersion")
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	sortField := q.Get("sortField")

	var mods []string
	total := 0

	if sortField == "3" {
		// Incremental: LastUpdated-sorted search.
		mods = f.recentMods
		total = len(mods)
	} else {
		key := fmt.Sprintf("%d|%s", classID, version)
		if override, ok := f.totalOverrides[key]; ok {
			total = override
		} else {
			mods = f.modsByVersion[version]
			total = len(mods)
		}
		if version != "" && pageSize != 1 {
			f.requestedVersions = append(f.requestedVersions, version)
		}
	}

	if pageSize == 1 {
		// Probe request: only the total matters.
		mods = nil
		if total > 0 && len(f.modsByVersion[version]) > 0 {
			mods = f.modsByVersion[version][:1]
		}
	} else if len(mods) > pageSize {
		mods = mods[:pageSize]
	}

	data := "["
	for i, m := range mods {
		if i > 0 {
			data += ","
		}
		data += m
	}
	data += "]"

	_, _ = fmt.Fprintf(w, `{"data": %s, "pagination": {"index": 0, "pageSize": %d, "resultCount": %d, "totalCount": %d}}`,
		data, pageSize, len(mods), total)
}

func setupSyncTest(t *testing.T, api *fakeAPI) (*Scraper, *store2.SQLiteStore) {
	t.Helper()
	srv := httptest.NewServer(api.handler())
	t.Cleanup(srv.Close)

	c, err := curseforge2.New(curseforge2.Config{
		BaseURL:           srv.URL,
		APIKey:            "test-key",
		RequestsPerSecond: 1000,
	})
	if err != nil {
		t.Fatalf("curseforge.New() error: %v", err)
	}
	t.Cleanup(c.Close)

	db, err := store2.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	badger, err := store2.NewBadgerStore(t.Context(), filepath.Join(t.TempDir(), "badger"))
	if err != nil {
		t.Fatalf("store.NewBadgerStore() error: %v", err)
	}
	t.Cleanup(func() { _ = badger.Close() })

	return New(c, db, badger, 50, slog.Default()), db
}

func TestFullSyncPartitionsAroundResultCap(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	api := &fakeAPI{
		modsByVersion: map[string][]string{
			"1.20.1": {modJSON(101, curseforge2.ClassMods, old), modJSON(102, curseforge2.ClassMods, old)},
			"1.19.2": {modJSON(103, curseforge2.ClassMods, old)},
		},
		// Class-level query exceeds the API result cap and must be subdivided
		// by game version.
		totalOverrides: map[string]int{"6|": maxSearchResults + 1},
	}
	s, db := setupSyncTest(t, api)

	err := s.Sync(context.Background(), SyncOptions{
		Full:    true,
		Classes: []curseforge2.ClassInfo{{ID: curseforge2.ClassMods, Name: "Mods", Slug: "mods"}},
	})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	stats, err := db.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}
	if stats.TotalMods != 3 {
		t.Errorf("TotalMods = %d, want 3", stats.TotalMods)
	}
	if stats.Categories != 1 {
		t.Errorf("Categories = %d, want 1", stats.Categories)
	}
	if _, ok := stats.SyncWatermarks["sync.class.6.last_update"]; !ok {
		t.Error("watermark for class 6 not stored")
	}

	// The snapshot version must not be used for partitioning by default.
	for _, v := range api.requestedVersions {
		if v == "23w13a" {
			t.Error("snapshot version 23w13a used in partition, want releases only")
		}
	}
	if len(api.requestedVersions) != 2 {
		t.Errorf("fetched partitions for %v, want 2 release versions", api.requestedVersions)
	}
}

func TestFullSyncAllVersionsIncludesSnapshots(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	api := &fakeAPI{
		modsByVersion: map[string][]string{
			"1.20.1": {modJSON(101, curseforge2.ClassMods, old)},
			"1.19.2": {modJSON(102, curseforge2.ClassMods, old)},
			"23w13a": {modJSON(103, curseforge2.ClassMods, old)},
		},
		totalOverrides: map[string]int{"6|": maxSearchResults + 1},
	}
	s, db := setupSyncTest(t, api)

	err := s.Sync(context.Background(), SyncOptions{
		Full:        true,
		AllVersions: true,
		Classes:     []curseforge2.ClassInfo{{ID: curseforge2.ClassMods, Name: "Mods", Slug: "mods"}},
	})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	stats, err := db.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}
	if stats.TotalMods != 3 {
		t.Errorf("TotalMods = %d, want 3 (all versions)", stats.TotalMods)
	}
}

func TestSmallCatalogSyncsWithoutPartitioning(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// No totalOverrides: class-level total is the version-less fixture size,
	// below the cap, so no version partition should happen.
	api := &fakeAPI{
		modsByVersion: map[string][]string{
			"": {modJSON(101, curseforge2.ClassWorlds, old), modJSON(102, curseforge2.ClassWorlds, old)},
		},
	}
	s, db := setupSyncTest(t, api)

	err := s.Sync(context.Background(), SyncOptions{
		Full:    true,
		Classes: []curseforge2.ClassInfo{{ID: curseforge2.ClassWorlds, Name: "Worlds", Slug: "worlds"}},
	})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	stats, err := db.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}
	if stats.TotalMods != 2 {
		t.Errorf("TotalMods = %d, want 2", stats.TotalMods)
	}
	if len(api.requestedVersions) != 0 {
		t.Errorf("version partitions requested %v, want none", api.requestedVersions)
	}
}

func TestIncrementalSyncOnlyFetchesUpdates(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	api := &fakeAPI{
		modsByVersion: map[string][]string{
			"": {modJSON(101, curseforge2.ClassMods, old)},
		},
	}
	s, db := setupSyncTest(t, api)
	ctx := context.Background()
	class := curseforge2.ClassInfo{ID: curseforge2.ClassMods, Name: "Mods", Slug: "mods"}

	if err := s.Sync(ctx, SyncOptions{Full: true, Classes: []curseforge2.ClassInfo{class}}); err != nil {
		t.Fatalf("full Sync() error: %v", err)
	}

	// A mod updated after the first sync's watermark.
	updated := time.Now().Add(2 * time.Hour).UTC()
	api.mu.Lock()
	api.recentMods = []string{
		modJSON(102, curseforge2.ClassMods, updated),
		modJSON(101, curseforge2.ClassMods, old),
	}
	api.mu.Unlock()

	// Second sync without --full must be incremental.
	if err := s.Sync(ctx, SyncOptions{Full: false, Classes: []curseforge2.ClassInfo{class}}); err != nil {
		t.Fatalf("incremental Sync() error: %v", err)
	}

	stats, err := db.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}
	if stats.TotalMods != 2 {
		t.Errorf("TotalMods = %d, want 2 (old + newly updated)", stats.TotalMods)
	}
}

func TestSubdivide(t *testing.T) {
	versions := []curseforge2.GameVersion{{Name: "1.20.1"}, {Name: "1.19.2"}}
	categories := []curseforge2.Category{{ID: 417}, {ID: 418}}
	mods := curseforge2.ClassInfo{ID: curseforge2.ClassMods, Name: "Mods", Slug: "mods"}
	packs := curseforge2.ClassInfo{ID: curseforge2.ClassResourcePacks, Name: "Resource Packs", Slug: "resource-packs"}

	tests := []struct {
		name      string
		p         partition
		class     curseforge2.ClassInfo
		want      []partition
		wantEmpty bool
	}{
		{
			name:  "class level splits by version",
			p:     partition{classID: curseforge2.ClassMods},
			class: mods,
			want: []partition{
				{classID: curseforge2.ClassMods, version: "1.20.1"},
				{classID: curseforge2.ClassMods, version: "1.19.2"},
			},
		},
		{
			name:  "version splits by loader for loader classes",
			p:     partition{classID: curseforge2.ClassMods, version: "1.20.1"},
			class: mods,
			want: []partition{
				{classID: curseforge2.ClassMods, version: "1.20.1", loader: 1},
				{classID: curseforge2.ClassMods, version: "1.20.1", loader: 2},
				{classID: curseforge2.ClassMods, version: "1.20.1", loader: 3},
				{classID: curseforge2.ClassMods, version: "1.20.1", loader: 4},
				{classID: curseforge2.ClassMods, version: "1.20.1", loader: 5},
				{classID: curseforge2.ClassMods, version: "1.20.1", loader: 6},
			},
		},
		{
			name:  "version splits by category for non-loader classes",
			p:     partition{classID: curseforge2.ClassResourcePacks, version: "1.20.1"},
			class: packs,
			want: []partition{
				{classID: curseforge2.ClassResourcePacks, version: "1.20.1", categoryID: 417},
				{classID: curseforge2.ClassResourcePacks, version: "1.20.1", categoryID: 418},
			},
		},
		{
			name:  "version and loader split by category",
			p:     partition{classID: curseforge2.ClassMods, version: "1.20.1", loader: 1},
			class: mods,
			want: []partition{
				{classID: curseforge2.ClassMods, version: "1.20.1", loader: 1, categoryID: 417},
				{classID: curseforge2.ClassMods, version: "1.20.1", loader: 1, categoryID: 418},
			},
		},
		{
			name:      "fully qualified partition cannot be subdivided",
			p:         partition{classID: curseforge2.ClassMods, version: "1.20.1", loader: 1, categoryID: 417},
			class:     mods,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subdivide(tt.p, tt.class, versions, categories)
			if tt.wantEmpty {
				if len(got) != 0 {
					t.Errorf("subdivide() = %v, want empty", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("subdivide() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("subdivide()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
