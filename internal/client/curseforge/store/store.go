package store

import (
	"context"

	"github.com/eldius/cli-telemetry-poc/internal/client/curseforge"
)

// Store represents a storage interface for managing mods and categories
type Store interface {
	Close() error
	UpsertMods(ctx context.Context, mods []curseforge.ModEnvelope) error
	UpsertCategories(ctx context.Context, categories []curseforge.Category) error
	GetSyncState(ctx context.Context, key string) (string, bool, error)
	SetSyncState(ctx context.Context, key, value string) error
	Stats(ctx context.Context) (*Stats, error)
	SearchMods(ctx context.Context, filter SearchFilter) ([]curseforge.ModEnvelope, error)
}

// DefaultSearchLimit caps result counts when SearchFilter.Limit is unset.
const DefaultSearchLimit = 20

// SearchFilter narrows a local mod search. The zero value matches every
// stored mod (up to the limit).
type SearchFilter struct {
	// Query is a case-insensitive substring matched against the mod name,
	// slug and summary. Empty matches all mods.
	Query string
	// ClassID restricts results to a content class (curseforge.Class*); 0 is any.
	ClassID int
	// GameVersion restricts results to mods whose latest files index this
	// exact Minecraft version (e.g. "1.20.1"); empty is any.
	GameVersion string
	// ModLoader restricts results to mods whose latest files index this
	// loader (curseforge.ModLoader*); curseforge.ModLoaderAny (0) is any.
	ModLoader int
	// Limit caps the number of results; <= 0 uses DefaultSearchLimit.
	Limit int
}

// limitOrDefault returns f.Limit or DefaultSearchLimit when unset.
func (f SearchFilter) limitOrDefault() int {
	if f.Limit <= 0 {
		return DefaultSearchLimit
	}
	return f.Limit
}
