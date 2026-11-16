package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eldius/cli-telemetry-poc/client/curseforge"
)

// SearchMods searches the stored mods, most downloaded first. Results are
// decoded from the lossless raw_json payload. GameVersion and ModLoader match
// against latest_files_indexes_json, so they only consider the files
// CurseForge reports as latest.
func (d *SQLiteStore) SearchMods(ctx context.Context, filter SearchFilter) ([]curseforge.ModEnvelope, error) {
	var (
		where []string
		args  []any
	)
	if filter.Query != "" {
		pattern := "%" + likeEscape(filter.Query) + "%"
		// SQLite LIKE is case-insensitive for ASCII by default.
		where = append(where, "(name LIKE ? ESCAPE '\\' OR slug LIKE ? ESCAPE '\\' OR summary LIKE ? ESCAPE '\\')")
		args = append(args, pattern, pattern, pattern)
	}
	if filter.ClassID != 0 {
		where = append(where, "class_id = ?")
		args = append(args, filter.ClassID)
	}
	if filter.GameVersion != "" {
		where = append(where, `EXISTS (
	SELECT 1 FROM json_each(mods.latest_files_indexes_json)
	WHERE json_extract(value, '$.gameVersion') = ?
)`)
		args = append(args, filter.GameVersion)
	}
	if filter.ModLoader != 0 {
		where = append(where, `EXISTS (
	SELECT 1 FROM json_each(mods.latest_files_indexes_json)
	WHERE json_extract(value, '$.modLoader') = ?
)`)
		args = append(args, filter.ModLoader)
	}

	query := `SELECT raw_json FROM mods`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY download_count DESC LIMIT ?"
	args = append(args, filter.limitOrDefault())

	var raws []string
	if err := d.db.SelectContext(ctx, &raws, query, args...); err != nil {
		return nil, fmt.Errorf("searching mods: %w", err)
	}

	mods := make([]curseforge.ModEnvelope, 0, len(raws))
	for _, raw := range raws {
		env := curseforge.ModEnvelope{Raw: json.RawMessage(raw)}
		if err := json.Unmarshal([]byte(raw), &env.Mod); err != nil {
			return nil, fmt.Errorf("decoding stored mod payload: %w", err)
		}
		mods = append(mods, env)
	}
	return mods, nil
}

// likeEscape escapes the LIKE wildcard characters in s so user input is
// matched literally (used with ESCAPE '\').
func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
