package curseforge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := New(Config{
		BaseURL:           srv.URL,
		APIKey:            "test-key",
		RequestsPerSecond: 1000,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

const sampleModJSON = `{
	"id": 238222,
	"gameId": 432,
	"name": "Just Enough Items (JEI)",
	"slug": "jei",
	"links": {"websiteUrl": "https://www.curseforge.com/minecraft/mc-mods/jei", "sourceUrl": "https://example.com/src"},
	"summary": "JEI is an item and recipe viewing mod",
	"status": 4,
	"downloadCount": 300000000,
	"isFeatured": true,
	"primaryCategoryId": 417,
	"categories": [{"id": 417, "gameId": 432, "name": "Map and Information", "slug": "map-information", "classId": 6}],
	"classId": 6,
	"authors": [{"id": 1, "name": "mezz", "url": "https://www.curseforge.com/members/mezz"}],
	"logo": {"id": 10, "modId": 238222, "url": "https://example.com/logo.png", "thumbnailUrl": "https://example.com/logo-thumb.png"},
	"screenshots": [],
	"mainFileId": 5001,
	"latestFiles": [{"id": 5001, "gameId": 432, "modId": 238222, "displayName": "jei-1.20.1.jar", "fileName": "jei-1.20.1.jar", "releaseType": 1, "fileDate": "2024-01-01T00:00:00Z", "gameVersions": ["1.20.1"], "isAvailable": true}],
	"latestFilesIndexes": [{"gameVersion": "1.20.1", "fileId": 5001, "filename": "jei-1.20.1.jar", "releaseType": 1, "modLoader": 1}],
	"dateCreated": "2014-11-10T00:00:00Z",
	"dateModified": "2024-06-01T00:00:00Z",
	"dateReleased": "2014-11-10T00:00:00Z",
	"allowModDistribution": true,
	"gamePopularityRank": 1,
	"isAvailable": true,
	"thumbsUpCount": 1234
}`

func TestSearchMods(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mods/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want %q", got, "test-key")
		}
		q := r.URL.Query()
		wantParams := map[string]string{
			"gameId":        "432",
			"classId":       "6",
			"pageSize":      "50",
			"index":         "50",
			"sortField":     "3",
			"sortOrder":     "desc",
			"gameVersion":   "1.20.1",
			"modLoaderType": "1",
		}
		for k, want := range wantParams {
			if got := q.Get(k); got != want {
				t.Errorf("param %s = %q, want %q", k, got, want)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data": [%s, %s], "pagination": {"index": 50, "pageSize": 50, "resultCount": 2, "totalCount": 152}}`, sampleModJSON, sampleModJSON)
	})

	c := testClient(t, handler)
	res, err := c.SearchMods(context.Background(), SearchParams{
		ClassID:       ClassMods,
		GameVersion:   "1.20.1",
		ModLoaderType: ModLoaderForge,
		SortField:     SortFieldLastUpdated,
		SortOrder:     SortOrderDesc,
		Index:         50,
		PageSize:      50,
	})
	if err != nil {
		t.Fatalf("SearchMods() error: %v", err)
	}

	if res.Pagination.TotalCount != 152 || res.Pagination.Index != 50 {
		t.Errorf("unexpected pagination: %+v", res.Pagination)
	}
	if len(res.Mods) != 2 {
		t.Fatalf("got %d mods, want 2", len(res.Mods))
	}
	m := res.Mods[0]
	if m.Mod.ID != 238222 || m.Mod.Slug != "jei" || m.Mod.ClassID != ClassMods {
		t.Errorf("unexpected mod: %+v", m.Mod)
	}
	if m.Mod.DownloadCount != 300000000 {
		t.Errorf("DownloadCount = %d", m.Mod.DownloadCount)
	}
	if m.Mod.Links.WebsiteURL != "https://www.curseforge.com/minecraft/mc-mods/jei" {
		t.Errorf("WebsiteURL = %q", m.Mod.Links.WebsiteURL)
	}
	if m.Mod.Logo == nil || m.Mod.Logo.ThumbnailURL == "" {
		t.Errorf("logo not decoded: %+v", m.Mod.Logo)
	}
	if len(m.Mod.LatestFiles) != 1 || m.Mod.LatestFiles[0].FileName != "jei-1.20.1.jar" {
		t.Errorf("latest files not decoded: %+v", m.Mod.LatestFiles)
	}
	if got := m.Mod.DateModified.Format("2006-01-02"); got != "2024-06-01" {
		t.Errorf("DateModified = %q", got)
	}

	// Raw payload must round-trip losslessly.
	var raw map[string]any
	if err := json.Unmarshal(m.Raw, &raw); err != nil {
		t.Fatalf("raw mod payload is not valid JSON: %v", err)
	}
	if raw["name"] != "Just Enough Items (JEI)" {
		t.Errorf("raw name = %v", raw["name"])
	}
}

func TestSearchModsDefaultParams(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("pageSize"); got != "50" {
			t.Errorf("default pageSize = %q, want 50", got)
		}
		if got := q.Get("sortField"); got != "2" {
			t.Errorf("default sortField = %q, want 2 (popularity)", got)
		}
		if got := q.Get("classId"); got != "" {
			t.Errorf("classId should be omitted, got %q", got)
		}
		_, _ = fmt.Fprint(w, `{"data": [], "pagination": {"index": 0, "pageSize": 50, "resultCount": 0, "totalCount": 0}}`)
	})

	c := testClient(t, handler)
	if _, err := c.SearchMods(context.Background(), SearchParams{}); err != nil {
		t.Fatalf("SearchMods() error: %v", err)
	}
}

func TestGetCategories(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/categories" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("classId"); got != "6" {
			t.Errorf("classId = %q, want 6", got)
		}
		_, _ = fmt.Fprint(w, `{"data": [
			{"id": 417, "gameId": 432, "name": "Map and Information", "slug": "map-information", "classId": 6, "isClass": false},
			{"id": 6, "gameId": 432, "name": "Mods", "slug": "mc-mods", "isClass": true}
		]}`)
	})

	c := testClient(t, handler)
	cats, err := c.GetCategories(context.Background(), ClassMods)
	if err != nil {
		t.Fatalf("GetCategories() error: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("got %d categories, want 2", len(cats))
	}
	if cats[1].IsClass != true || cats[1].ID != ClassMods {
		t.Errorf("unexpected class category: %+v", cats[1])
	}
}

func TestGetGameVersions(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/games/432/versions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data": [
			{"id": 1, "gameId": 432, "name": "1.20.1", "slug": "1-20-1", "gameVersionTypeId": 100},
			{"id": 2, "gameId": 432, "name": "23w13a", "slug": "23w13a", "gameVersionTypeId": 100}
		]}`)
	})

	c := testClient(t, handler)
	versions, err := c.GetGameVersions(context.Background())
	if err != nil {
		t.Fatalf("GetGameVersions() error: %v", err)
	}
	if len(versions) != 2 || versions[0].Name != "1.20.1" {
		t.Errorf("unexpected versions: %+v", versions)
	}
}

func TestRetryOnServerError(t *testing.T) {
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"errorMessage": "boom"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"data": [], "pagination": {"index": 0, "pageSize": 50, "resultCount": 0, "totalCount": 0}}`)
	})

	c := testClient(t, handler)
	start := time.Now()
	if _, err := c.SearchMods(context.Background(), SearchParams{}); err != nil {
		t.Fatalf("SearchMods() error: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("got %d calls, want 2", calls.Load())
	}
	if elapsed := time.Since(start); elapsed < initialBackoff {
		t.Errorf("retry happened too fast (%s), backoff not applied", elapsed)
	}
}

func TestNonRetryableError(t *testing.T) {
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"errorMessage": "invalid api key"}`)
	})

	c := testClient(t, handler)
	_, err := c.SearchMods(context.Background(), SearchParams{})
	if err == nil {
		t.Fatal("SearchMods() expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not an *APIError: %T", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want 403", apiErr.StatusCode)
	}
	if calls.Load() != 1 {
		t.Errorf("got %d calls, want 1 (no retries on 403)", calls.Load())
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrAPIKeyRequired) {
		t.Errorf("New() error = %v, want ErrAPIKeyRequired", err)
	}
}

func TestResolveClasses(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantIDs []int
		wantErr bool
	}{
		{name: "by slug", input: []string{"mods", "modpacks"}, wantIDs: []int{ClassMods, ClassModpacks}},
		{name: "by name case insensitive", input: []string{"Mods", "Resource Packs"}, wantIDs: []int{ClassMods, ClassResourcePacks}},
		{name: "numeric id", input: []string{"6"}, wantIDs: []int{ClassMods}},
		{name: "unknown numeric id is accepted", input: []string{"9999"}, wantIDs: []int{9999}},
		{name: "unknown name", input: []string{"nope"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveClasses(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveClasses() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("ResolveClasses() = %+v, want IDs %v", got, tt.wantIDs)
			}
			for i, id := range tt.wantIDs {
				if got[i].ID != id {
					t.Errorf("ResolveClasses()[%d].ID = %d, want %d", i, got[i].ID, id)
				}
			}
		})
	}
}

func TestClassHelpers(t *testing.T) {
	if got := ClassName(ClassMods); got != "Mods" {
		t.Errorf("ClassName(ClassMods) = %q", got)
	}
	if got := ClassName(4242); got != "class-4242" {
		t.Errorf("ClassName(4242) = %q", got)
	}
	if !ClassUsesLoaders(ClassMods) || !ClassUsesLoaders(ClassModpacks) {
		t.Error("mods and modpacks should use loaders")
	}
	if ClassUsesLoaders(ClassResourcePacks) {
		t.Error("resource packs should not use loaders")
	}
	if strconv.Itoa(ClassMods) != "6" {
		t.Errorf("ClassMods = %d, want 6", ClassMods)
	}
}
