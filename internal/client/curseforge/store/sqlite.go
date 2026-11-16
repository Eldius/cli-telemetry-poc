// Package store implements the local SQLite cache/database for scraped
// CurseForge content.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eldius/cli-telemetry-poc/internal/client/curseforge"
	"github.com/eldius/cli-telemetry-poc/pkg/db"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // SQLite driver
)

var _ Store = &SQLiteStore{}

//go:embed schema.sql
var schema string

// ClassStat summarizes stored mods for one content class.
type ClassStat struct {
	ClassID    int
	Count      int
	LastSynced time.Time
}

// Stats summarize the database contents.
type Stats struct {
	Classes        []ClassStat
	TotalMods      int
	Categories     int
	SyncWatermarks map[string]string
}

// SQLiteStore is the local SQLite store.
type SQLiteStore struct {
	db *sqlx.DB
}

// Open opens (creating if needed) the SQLite database at path and ensures
// the schema exists.
func Open(path string) (*SQLiteStore, error) {
	_ = os.MkdirAll(filepath.Dir(path), 0o750)

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	sqlDB, err := db.GetSqlxDB("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database %q: %w", path, err)
	}
	// SQLite handles a single writer; avoid "database is locked" errors.
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(schema); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}
	return &SQLiteStore{db: sqlDB}, nil
}

// Close closes the database.
func (d *SQLiteStore) Close() error {
	return d.db.Close()
}

const upsertModSQL = `
INSERT INTO mods (
    id, game_id, class_id, name, slug, summary, status, download_count,
    is_featured, primary_category_id, website_url, source_url, issues_url,
    wiki_url, logo_url, logo_thumbnail_url, main_file_id, date_created,
    date_modified, date_released, allow_mod_distribution, game_popularity_rank,
    is_available, thumbs_up_count, categories_json, authors_json,
    latest_files_json, latest_files_indexes_json, raw_json, synced_at
) VALUES (
    :id, :game_id, :class_id, :name, :slug, :summary, :status, :download_count,
    :is_featured, :primary_category_id, :website_url, :source_url, :issues_url,
    :wiki_url, :logo_url, :logo_thumbnail_url, :main_file_id, :date_created,
    :date_modified, :date_released, :allow_mod_distribution, :game_popularity_rank,
    :is_available, :thumbs_up_count, :categories_json, :authors_json,
    :latest_files_json, :latest_files_indexes_json, :raw_json, :synced_at
)
ON CONFLICT (id) DO UPDATE SET
    game_id = excluded.game_id,
    class_id = excluded.class_id,
    name = excluded.name,
    slug = excluded.slug,
    summary = excluded.summary,
    status = excluded.status,
    download_count = excluded.download_count,
    is_featured = excluded.is_featured,
    primary_category_id = excluded.primary_category_id,
    website_url = excluded.website_url,
    source_url = excluded.source_url,
    issues_url = excluded.issues_url,
    wiki_url = excluded.wiki_url,
    logo_url = excluded.logo_url,
    logo_thumbnail_url = excluded.logo_thumbnail_url,
    main_file_id = excluded.main_file_id,
    date_created = excluded.date_created,
    date_modified = excluded.date_modified,
    date_released = excluded.date_released,
    allow_mod_distribution = excluded.allow_mod_distribution,
    game_popularity_rank = excluded.game_popularity_rank,
    is_available = excluded.is_available,
    thumbs_up_count = excluded.thumbs_up_count,
    categories_json = excluded.categories_json,
    authors_json = excluded.authors_json,
    latest_files_json = excluded.latest_files_json,
    latest_files_indexes_json = excluded.latest_files_indexes_json,
    raw_json = excluded.raw_json,
    synced_at = excluded.synced_at
`

// UpsertMods inserts or updates a batch of mods in a single transaction.
func (d *SQLiteStore) UpsertMods(ctx context.Context, mods []curseforge.ModEnvelope) error {
	if len(mods) == 0 {
		return nil
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning mods upsert transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareNamedContext(ctx, upsertModSQL)
	if err != nil {
		return fmt.Errorf("preparing mods upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC()
	for _, env := range mods {
		row, err := modRow(env, now)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, row); err != nil {
			return fmt.Errorf("upserting mod %d (%s): %w", env.Mod.ID, env.Mod.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing mods upsert: %w", err)
	}
	return nil
}

func modRow(env curseforge.ModEnvelope, syncedAt time.Time) (map[string]any, error) {
	m := env.Mod

	categoriesJSON, err := marshalJSON(m.Categories)
	if err != nil {
		return nil, fmt.Errorf("marshaling categories for mod %d: %w", m.ID, err)
	}
	authorsJSON, err := marshalJSON(m.Authors)
	if err != nil {
		return nil, fmt.Errorf("marshaling authors for mod %d: %w", m.ID, err)
	}
	latestFilesJSON, err := marshalJSON(m.LatestFiles)
	if err != nil {
		return nil, fmt.Errorf("marshaling latest files for mod %d: %w", m.ID, err)
	}
	latestFilesIndexesJSON, err := marshalJSON(m.LatestFilesIndexes)
	if err != nil {
		return nil, fmt.Errorf("marshaling latest files indexes for mod %d: %w", m.ID, err)
	}

	var logoURL, logoThumbnailURL string
	if m.Logo != nil {
		logoURL = m.Logo.URL
		logoThumbnailURL = m.Logo.ThumbnailURL
	}

	return map[string]any{
		"id":                        m.ID,
		"game_id":                   m.GameID,
		"class_id":                  m.ClassID,
		"name":                      m.Name,
		"slug":                      m.Slug,
		"summary":                   m.Summary,
		"status":                    m.Status,
		"download_count":            m.DownloadCount,
		"is_featured":               m.IsFeatured,
		"primary_category_id":       m.PrimaryCategoryID,
		"website_url":               m.Links.WebsiteURL,
		"source_url":                m.Links.SourceURL,
		"issues_url":                m.Links.IssuesURL,
		"wiki_url":                  m.Links.WikiURL,
		"logo_url":                  logoURL,
		"logo_thumbnail_url":        logoThumbnailURL,
		"main_file_id":              m.MainFileID,
		"date_created":              rfc3339(m.DateCreated),
		"date_modified":             rfc3339(m.DateModified),
		"date_released":             rfc3339(m.DateReleased),
		"allow_mod_distribution":    m.AllowModDistribution,
		"game_popularity_rank":      m.GamePopularityRank,
		"is_available":              m.IsAvailable,
		"thumbs_up_count":           m.ThumbsUpCount,
		"categories_json":           categoriesJSON,
		"authors_json":              authorsJSON,
		"latest_files_json":         latestFilesJSON,
		"latest_files_indexes_json": latestFilesIndexesJSON,
		"raw_json":                  string(env.Raw),
		"synced_at":                 rfc3339(syncedAt),
	}, nil
}

const upsertCategorySQL = `
INSERT INTO categories (
    id, game_id, class_id, parent_category_id, name, slug, url, icon_url,
    is_class, date_modified, raw_json, synced_at
) VALUES (
    :id, :game_id, :class_id, :parent_category_id, :name, :slug, :url, :icon_url,
    :is_class, :date_modified, :raw_json, :synced_at
)
ON CONFLICT (id) DO UPDATE SET
    game_id = excluded.game_id,
    class_id = excluded.class_id,
    parent_category_id = excluded.parent_category_id,
    name = excluded.name,
    slug = excluded.slug,
    url = excluded.url,
    icon_url = excluded.icon_url,
    is_class = excluded.is_class,
    date_modified = excluded.date_modified,
    raw_json = excluded.raw_json,
    synced_at = excluded.synced_at
`

// UpsertCategories inserts or updates a batch of categories in a single
// transaction.
func (d *SQLiteStore) UpsertCategories(ctx context.Context, categories []curseforge.Category) error {
	if len(categories) == 0 {
		return nil
	}
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning categories upsert transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareNamedContext(ctx, upsertCategorySQL)
	if err != nil {
		return fmt.Errorf("preparing categories upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC()
	for _, c := range categories {
		raw, err := json.Marshal(c)
		if err != nil {
			return fmt.Errorf("marshaling category %d: %w", c.ID, err)
		}
		if _, err := stmt.ExecContext(ctx, map[string]any{
			"id":                 c.ID,
			"game_id":            c.GameID,
			"class_id":           c.ClassID,
			"parent_category_id": c.ParentCategoryID,
			"name":               c.Name,
			"slug":               c.Slug,
			"url":                c.URL,
			"icon_url":           c.IconURL,
			"is_class":           c.IsClass,
			"date_modified":      rfc3339(c.DateModified),
			"raw_json":           string(raw),
			"synced_at":          rfc3339(now),
		}); err != nil {
			return fmt.Errorf("upserting category %d (%s): %w", c.ID, c.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing categories upsert: %w", err)
	}
	return nil
}

// GetSyncState returns the value stored for key, and whether it exists.
func (d *SQLiteStore) GetSyncState(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := d.db.GetContext(ctx, &value, `SELECT value FROM sync_state WHERE key = ?`, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("getting sync state %q: %w", key, err)
	}
	return value, true, nil
}

// SetSyncState stores value under key.
func (d *SQLiteStore) SetSyncState(ctx context.Context, key, value string) error {
	_, err := d.db.ExecContext(ctx, `
INSERT INTO sync_state (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`, key, value)
	if err != nil {
		return fmt.Errorf("setting sync state %q: %w", key, err)
	}
	return nil
}

// Stats returns a summary of the database contents.
func (d *SQLiteStore) Stats(ctx context.Context) (*Stats, error) {
	var stats Stats

	rows := []struct {
		ClassID    int     `db:"class_id"`
		Count      int     `db:"count"`
		LastSynced *string `db:"last_synced"`
	}{}
	if err := d.db.SelectContext(ctx, &rows, `
SELECT class_id, COUNT(*) AS count, MAX(synced_at) AS last_synced
FROM mods GROUP BY class_id ORDER BY class_id
`); err != nil {
		return nil, fmt.Errorf("querying class stats: %w", err)
	}
	for _, r := range rows {
		st := ClassStat{ClassID: r.ClassID, Count: r.Count}
		if r.LastSynced != nil {
			st.LastSynced = parseSQLiteTime(*r.LastSynced)
		}
		stats.Classes = append(stats.Classes, st)
		stats.TotalMods += r.Count
	}

	if err := d.db.GetContext(ctx, &stats.Categories, `SELECT COUNT(*) FROM categories`); err != nil {
		return nil, fmt.Errorf("querying categories count: %w", err)
	}

	watermarks := map[string]string{}
	wmRows := []struct {
		Key   string `db:"key"`
		Value string `db:"value"`
	}{}
	if err := d.db.SelectContext(ctx, &wmRows, `SELECT key, value FROM sync_state ORDER BY key`); err != nil {
		return nil, fmt.Errorf("querying sync state: %w", err)
	}
	for _, r := range wmRows {
		watermarks[r.Key] = r.Value
	}
	stats.SyncWatermarks = watermarks

	return &stats, nil
}

// sqliteTimeLayouts are the timestamp formats values may be stored in:
// RFC3339 for values written by this tool, "2006-01-02 15:04:05" for
// CURRENT_TIMESTAMP defaults, and Go's default time.Time String format for
// values bound by the driver.
var sqliteTimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
}

// rfc3339 formats t as an RFC3339 UTC timestamp for storage.
func rfc3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// parseSQLiteTime parses a stored timestamp, returning the zero time on
// failure.
func parseSQLiteTime(s string) time.Time {
	for _, layout := range sqliteTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
