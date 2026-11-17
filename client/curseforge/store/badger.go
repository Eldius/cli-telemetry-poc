package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/eldius/cli-telemetry-poc/client/curseforge"
	badgerhold "github.com/timshannon/badgerhold/v4"
)

var (
	_ Store         = &BadgerStore{}
	_ badger.Logger = &badgerLogger{}
)

// BadgerStore is a store implementation using BadgerDB
type BadgerStore struct {
	store *badgerhold.Store
}

// Close closes the BadgerDB store
func (b *BadgerStore) Close() error {
	return b.store.Close()
}

// UpsertMods upserts mods into the BadgerDB store
func (b *BadgerStore) UpsertMods(_ context.Context, mods []curseforge.ModEnvelope) error {
	tx := b.store.Badger().NewTransaction(true)
	defer tx.Discard()
	for _, m := range mods {
		err := b.store.TxUpsert(tx, m.Mod.ID, m)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertCategories upserts categories into the BadgerDB store
func (b *BadgerStore) UpsertCategories(_ context.Context, categories []curseforge.Category) error {
	tx := b.store.Badger().NewTransaction(true)
	defer tx.Discard()
	for _, c := range categories {
		err := b.store.TxUpsert(tx, c.ID, c)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSyncState retrieves the sync state from the BadgerDB store
func (b *BadgerStore) GetSyncState(_ context.Context, key string) (string, bool, error) {
	var value string
	if err := b.store.Get(key, &value); err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetSyncState sets the sync state in the BadgerDB store
func (b *BadgerStore) SetSyncState(_ context.Context, key, value string) error {
	return b.store.Upsert(key, value)
}

// Stats retrieves statistics from the BadgerDB store
func (b *BadgerStore) Stats(_ context.Context) (*Stats, error) {
	return nil, nil
}

// SearchMods searches the mods stored in BadgerDB, most downloaded first.
// BadgerHold has no index for this query, so it scans every stored mod; that
// is acceptable while BadgerDB is an experimental mirror of the SQLite store.
func (b *BadgerStore) SearchMods(_ context.Context, filter SearchFilter) ([]curseforge.ModEnvelope, error) {
	query := strings.ToLower(filter.Query)
	var mods []curseforge.ModEnvelope
	err := b.store.Find(&mods, badgerhold.Where("Mod").MatchFunc(func(ra *badgerhold.RecordAccess) (bool, error) {
		var env curseforge.ModEnvelope
		switch r := ra.Record().(type) {
		case *curseforge.ModEnvelope:
			env = *r
		case curseforge.ModEnvelope:
			env = r
		default:
			return false, nil
		}
		return matchesFilter(env.Mod, query, filter), nil
	}))
	if err != nil {
		return nil, err
	}
	sort.Slice(mods, func(i, j int) bool {
		return mods[i].Mod.DownloadCount > mods[j].Mod.DownloadCount
	})
	if limit := filter.limitOrDefault(); len(mods) > limit {
		mods = mods[:limit]
	}
	return mods, nil
}

// matchesFilter reports whether m satisfies the search filter; query must
// already be lowercased.
func matchesFilter(m curseforge.Mod, query string, filter SearchFilter) bool {
	if query != "" &&
		!strings.Contains(strings.ToLower(m.Name), query) &&
		!strings.Contains(strings.ToLower(m.Slug), query) &&
		!strings.Contains(strings.ToLower(m.Summary), query) {
		return false
	}
	if filter.ClassID != 0 && m.ClassID != filter.ClassID {
		return false
	}
	if filter.GameVersion == "" && filter.ModLoader == 0 {
		return true
	}
	versionOK := filter.GameVersion == ""
	loaderOK := filter.ModLoader == 0
	for _, idx := range m.LatestFilesIndexes {
		versionOK = versionOK || idx.GameVersion == filter.GameVersion
		loaderOK = loaderOK || idx.ModLoader == filter.ModLoader
	}
	return versionOK && loaderOK
}

// NewBadgerStore creates a new BadgerDB store
func NewBadgerStore(ctx context.Context, path string) (*BadgerStore, error) {
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	options := badgerhold.DefaultOptions
	options.Dir = path
	options.ValueDir = path
	options.Logger = badgerLogger{l: slog.With("pkg", "badger"), ctx: ctx}
	store, err := badgerhold.Open(options)
	if err != nil {
		return nil, err
	}
	return &BadgerStore{store: store}, nil
}

var _ badger.Logger = &badgerLogger{}

type badgerLogger struct {
	l   *slog.Logger
	ctx context.Context
}

func (b badgerLogger) Errorf(s string, i ...any) {
	b.getLogger(s, i).ErrorContext(b.ctx, "BadgerStore")
}

func (b badgerLogger) getLogger(s string, i ...any) *slog.Logger {
	return b.l.With(slog.String("badger.msg", fmt.Sprintf(s, i...)))
}

func (b badgerLogger) Warningf(s string, i ...any) {
	b.getLogger(s, i).WarnContext(b.ctx, "BadgerStore")
}

func (b badgerLogger) Infof(s string, i ...any) {
	b.getLogger(s, i).InfoContext(b.ctx, "BadgerStore")
}

func (b badgerLogger) Debugf(s string, i ...any) {
	b.getLogger(s, i).DebugContext(b.ctx, "BadgerStore")
}
