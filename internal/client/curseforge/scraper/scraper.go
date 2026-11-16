// Package scraper orchestrates fetching content metadata from CurseForge
// and storing it in the local database.
package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/eldius/cli-telemetry-poc/internal/client/curseforge"
	"github.com/eldius/cli-telemetry-poc/internal/client/curseforge/store"
)

// maxSearchResults is the CurseForge search API cap: results beyond this
// index cannot be fetched, so larger result sets must be partitioned.
const maxSearchResults = 10000

// releaseVersion matches stable Minecraft release version names
// (e.g. "1.20.1", "1.20", "1.7.10").
var releaseVersion = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)

// SyncOptions configures a sync run.
type SyncOptions struct {
	// Full fetches the whole catalog (partitioning around the API result
	// cap). Otherwise only mods updated since the last sync are fetched.
	Full bool
	// AllVersions includes snapshots/pre-releases in full sync partitions
	// (by default only release versions are used).
	AllVersions bool
	// Classes are the content classes to sync.
	Classes []curseforge.ClassInfo
}

// Scraper fetches CurseForge content and stores it locally.
type Scraper struct {
	api      *curseforge.Client
	db       *store.SQLiteStore
	badger   *store.BadgerStore
	pageSize int
	log      *slog.Logger

	versions     []curseforge.GameVersion
	versionsLoad bool
}

// New creates a Scraper.
func New(api *curseforge.Client, db *store.SQLiteStore, badger *store.BadgerStore, pageSize int, log *slog.Logger) *Scraper {
	if pageSize <= 0 {
		pageSize = 50
	}
	return &Scraper{api: api, db: db, badger: badger, pageSize: pageSize, log: log}
}

// Sync runs a sync for all configured classes. It continues on per-class
// errors and reports the first error at the end.
func (s *Scraper) Sync(ctx context.Context, opts SyncOptions) error {
	//var firstErr error
	for _, class := range opts.Classes {
		if err := s.syncClass(ctx, class, opts); err != nil {
			s.log.With("error", err, "class_name", class.Name).Error("failed to sync class")
			return err
		}
	}
	//return firstErr
	return nil
}

func (s *Scraper) syncClass(ctx context.Context, class curseforge.ClassInfo, opts SyncOptions) error {
	log := s.log
	start := time.Now()
	log.With("class_id", class.ID, "clqaqss_name", class.Name).InfoContext(ctx, "syncing class")

	categories, err := s.api.GetCategories(ctx, class.ID)
	if err != nil {
		return fmt.Errorf("syncing class %s: fetching categories: %w", class.Name, err)
	}
	if err := s.db.UpsertCategories(ctx, categories); err != nil {
		return fmt.Errorf("syncing class %s: storing categories: %w", class.Name, err)
	}
	if err := s.badger.UpsertCategories(ctx, categories); err != nil {
		return fmt.Errorf("syncing class %s: storing categories: %w", class.Name, err)
	}
	log.With("class_name", class.Name, "categories_count", len(categories)).Info("category stored")

	var count int
	watermark, hasWatermark, err := s.watermark(ctx, class)
	if err != nil {
		return err
	}

	if opts.Full || !hasWatermark {
		if !opts.Full {
			log.With("class_name", class.Name).Info("no previous sync found, running full sync")
		}
		count, err = s.syncClassFull(ctx, class, opts.AllVersions, categories)
	} else {
		count, err = s.syncClassIncremental(ctx, class, watermark)
	}
	if err != nil {
		return fmt.Errorf("syncing class %s: %w", class.Name, err)
	}

	if err := s.setWatermark(ctx, class, start); err != nil {
		return err
	}
	log.With("class_name", class.Name, "count", count, "processing_time", time.Since(start).Round(time.Second)).Info("mods processIng done")
	return nil
}

// syncClassFull fetches the whole catalog for a class, recursively
// partitioning queries that exceed the API result cap.
func (s *Scraper) syncClassFull(ctx context.Context, class curseforge.ClassInfo, allVersions bool, categories []curseforge.Category) (int, error) {
	versions, err := s.partitionVersions(ctx, allVersions)
	if err != nil {
		return 0, err
	}

	queue := []partition{{classID: class.ID}}
	total := 0
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]

		count, err := s.probe(ctx, p)
		if err != nil {
			return total, err
		}
		if count == 0 {
			continue
		}

		if count <= maxSearchResults {
			n, err := s.fetchPartition(ctx, p, count)
			total += n
			if err != nil {
				return total, err
			}
			continue
		}

		subs := subdivide(p, class, versions, categories)
		if len(subs) == 0 {
			s.log.With("partition", p, "count", count, "max_search_results", maxSearchResults).Warn("partition holds results but cannot be subdivided further; only the firsts will be fetched")
			n, err := s.fetchPartition(ctx, p, maxSearchResults)
			total += n
			if err != nil {
				return total, err
			}
			continue
		}
		s.log.With("partition", p, "count", count, "subs_count", len(subs)).Debug("partition holds results")
		queue = append(queue, subs...)
	}
	return total, nil
}

// syncClassIncremental fetches mods updated since the watermark, sorted by
// last update descending.
func (s *Scraper) syncClassIncremental(ctx context.Context, class curseforge.ClassInfo, watermark time.Time) (int, error) {
	total := 0
	index := 0
	for ; index < maxSearchResults; index += s.pageSize {
		res, err := s.api.SearchMods(ctx, curseforge.SearchParams{
			ClassID:   class.ID,
			SortField: curseforge.SortFieldLastUpdated,
			SortOrder: curseforge.SortOrderDesc,
			Index:     index,
			PageSize:  s.pageSize,
		})
		if err != nil {
			return total, err
		}

		fresh := res.Mods[:0]
		for _, env := range res.Mods {
			if env.Mod.DateModified.Before(watermark) {
				continue
			}
			fresh = append(fresh, env)
		}
		if err := s.db.UpsertMods(ctx, fresh); err != nil {
			return total, err
		}
		if err := s.badger.UpsertMods(ctx, fresh); err != nil {
			return total, err
		}
		total += len(fresh)

		if len(fresh) < len(res.Mods) || len(res.Mods) < s.pageSize {
			break
		}
	}
	if index >= maxSearchResults {
		s.log.With("class_name", class.Name, "max_search_results", maxSearchResults, "max_search_results_minus_total", maxSearchResults-total).Warn("class hit the result cap during incremental sync; %mods behind may be stale, consider a full sync")
	}
	return total, nil
}

// probe returns the total number of results for a partition.
func (s *Scraper) probe(ctx context.Context, p partition) (int, error) {
	res, err := s.api.SearchMods(ctx, p.searchParams(0, 1))
	if err != nil {
		return 0, fmt.Errorf("probing partition %s: %w", p, err)
	}
	return res.Pagination.TotalCount, nil
}

// fetchPartition fetches all pages of a partition (up to limit results) and
// upserts them into the store.
func (s *Scraper) fetchPartition(ctx context.Context, p partition, limit int) (int, error) {
	s.log.With("partition", p, "limit", limit).Info("fetching results")
	fetched := 0
	for index := 0; index < limit && index < maxSearchResults; index += s.pageSize {
		res, err := s.api.SearchMods(ctx, p.searchParams(index, s.pageSize))
		if err != nil {
			return fetched, fmt.Errorf("fetching partition %s at index %d: %w", p, index, err)
		}
		if err := s.db.UpsertMods(ctx, res.Mods); err != nil {
			return fetched, err
		}
		if err := s.badger.UpsertMods(ctx, res.Mods); err != nil {
			return fetched, err
		}
		fetched += len(res.Mods)
		if len(res.Mods) < s.pageSize {
			break
		}
	}
	s.log.With("fetch_count", fetched, "partition", p).Info("fetched %d mods for %s")
	return fetched, nil
}

// partitionVersions returns the game versions used to partition full syncs,
// lazily fetched and cached on the scraper.
func (s *Scraper) partitionVersions(ctx context.Context, allVersions bool) ([]curseforge.GameVersion, error) {
	if !s.versionsLoad {
		versions, err := s.api.GetGameVersions(ctx)
		if err != nil {
			return nil, err
		}
		s.versions = versions
		s.versionsLoad = true
	}
	if allVersions {
		return s.versions, nil
	}
	var releases []curseforge.GameVersion
	for _, v := range s.versions {
		if releaseVersion.MatchString(v.Name) {
			releases = append(releases, v)
		}
	}
	return releases, nil
}

func (s *Scraper) watermark(ctx context.Context, class curseforge.ClassInfo) (time.Time, bool, error) {
	value, ok, err := s.db.GetSyncState(ctx, watermarkKey(class))
	if err != nil {
		return time.Time{}, false, err
	}
	if !ok {
		return time.Time{}, false, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parsing watermark %q: %w", value, err)
	}
	return t, true, nil
}

func (s *Scraper) setWatermark(ctx context.Context, class curseforge.ClassInfo, t time.Time) error {
	if err := s.db.SetSyncState(ctx, watermarkKey(class), t.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("storing watermark for class %s: %w", class.Name, err)
	}
	return nil
}

func watermarkKey(class curseforge.ClassInfo) string {
	return fmt.Sprintf("sync.class.%d.last_update", class.ID)
}

// partition identifies one search query slice of a class catalog.
type partition struct {
	classID    int
	version    string
	loader     int
	categoryID int
}

func (p partition) searchParams(index, pageSize int) curseforge.SearchParams {
	return curseforge.SearchParams{
		ClassID:       p.classID,
		CategoryID:    p.categoryID,
		GameVersion:   p.version,
		ModLoaderType: p.loader,
		SortField:     curseforge.SortFieldPopularity,
		SortOrder:     curseforge.SortOrderDesc,
		Index:         index,
		PageSize:      pageSize,
	}
}

func (p partition) String() string {
	s := fmt.Sprintf("class %s", curseforge.ClassName(p.classID))
	if p.version != "" {
		s += ", version " + p.version
	}
	if p.loader != 0 {
		s += fmt.Sprintf(", loader %d", p.loader)
	}
	if p.categoryID != 0 {
		s += fmt.Sprintf(", category %d", p.categoryID)
	}
	return s
}

// subdivide splits a partition that exceeds the API result cap into smaller
// partitions: first by game version, then by mod loader (for loader-based
// classes), then by category. It returns nil when no further subdivision is
// possible.
func subdivide(p partition, class curseforge.ClassInfo, versions []curseforge.GameVersion, categories []curseforge.Category) []partition {
	if p.version == "" {
		subs := make([]partition, 0, len(versions))
		for _, v := range versions {
			subs = append(subs, partition{classID: p.classID, version: v.Name})
		}
		return subs
	}
	if p.loader == 0 && curseforge.ClassUsesLoaders(class.ID) {
		subs := make([]partition, 0, len(curseforge.LoaderIDs))
		for _, l := range curseforge.LoaderIDs {
			subs = append(subs, partition{classID: p.classID, version: p.version, loader: l})
		}
		return subs
	}
	if p.categoryID == 0 {
		subs := make([]partition, 0, len(categories))
		for _, c := range categories {
			subs = append(subs, partition{classID: p.classID, version: p.version, loader: p.loader, categoryID: c.ID})
		}
		return subs
	}
	return nil
}
