package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/eldius/cli-telemetry-poc/client/curseforge"
	"github.com/eldius/cli-telemetry-poc/client/curseforge/scraper"
	"github.com/eldius/cli-telemetry-poc/client/curseforge/store"
	"github.com/eldius/cli-telemetry-poc/pkg/http"
	"github.com/eldius/cli-telemetry-poc/pkg/telemetry"
	"github.com/spf13/cobra"
)

var (
	syncFull        bool
	syncAllVersions bool
	syncClasses     []string
	syncDBPath      string
)

// syncCmd fetches content metadata from CurseForge into the local database.
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch content metadata from CurseForge into the local database",
	Long: `Fetches Minecraft mods, modpacks, addons and other content metadata from
CurseForge and upserts it into the local SQLite database.

The first run performs a full sync per class (partitioning search queries to
work around the CurseForge 10k search result cap). Subsequent runs only fetch
mods updated since the last sync, unless --full is given.`,
	Example: `  cli-telemetry-poc sync                    # incremental sync of all classes
  cli-telemetry-poc sync --full             # full re-sync of all classes
  cli-telemetry-poc sync --classes mods,modpacks
  CURSEFORGE_API_KEY=xxx cli-telemetry-poc sync --full`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		log := slog.Default()

		api, err := newAPIClient()
		if err != nil {
			return telemetry.RecordErrorAndClose(ctx, err)
		}
		defer api.Close()

		db, err := openDatabase(syncDBPath)
		if err != nil {
			return telemetry.RecordErrorAndClose(ctx, err)
		}
		defer func() { _ = db.Close() }()

		badger, err := openBadger(ctx)
		if err != nil {
			return telemetry.RecordErrorAndClose(ctx, err)
		}
		defer func() { _ = badger.Close() }()

		classes, err := resolveClasses(syncClasses)
		if err != nil {
			return telemetry.RecordErrorAndClose(ctx, err)
		}

		s := scraper.New(api, db, badger, 0, log)
		if err := s.Sync(ctx, scraper.SyncOptions{
			Full:        syncFull,
			AllVersions: syncAllVersions,
			Classes:     classes,
		}); err != nil {
			return telemetry.RecordErrorAndClose(ctx, fmt.Errorf("sync: %w", err))
		}

		log.Info("sync finished")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().BoolVar(&syncFull, "full", false, "fetch the whole catalog even if a previous sync exists")
	syncCmd.Flags().BoolVar(&syncAllVersions, "all-versions", false, "partition full syncs by all game versions, including snapshots and pre-releases")
	syncCmd.Flags().StringSliceVar(&syncClasses, "classes", nil, "comma-separated content classes to sync (known: "+curseforge.KnownClassNames()+")")
	syncCmd.Flags().StringVar(&syncDBPath, "db", "", "database file path (default .data/mods.db)")
}

// newAPIClient builds a CurseForge API client. The API key comes from the
// CURSEFORGE_API_KEY environment variable.
func newAPIClient() (*curseforge.Client, error) {
	return curseforge.New(curseforge.Config{
		BaseURL:           "https://api.curseforge.com",
		APIKey:            os.Getenv("CURSEFORGE_API_KEY"),
		GameID:            432,
		RequestsPerSecond: 5,
		HTTPClient:        http.NewHTTPClient(),
	})
}

// openDatabase opens the store at the --db path, defaulting to
// .data/mods.db when the flag is empty.
func openDatabase(dbFlag string) (*store.SQLiteStore, error) {
	path := dbFlag
	if path == "" {
		path = ".data/mods.db"
	}
	return store.Open(path)
}

func openBadger(ctx context.Context) (*store.BadgerStore, error) {
	return store.NewBadgerStore(ctx, ".data/badger.db")
}

// resolveClasses parses the --classes flag into class infos, defaulting to
// all known classes when the flag is empty.
func resolveClasses(classesFlag []string) ([]curseforge.ClassInfo, error) {
	if len(classesFlag) == 0 {
		return curseforge.KnownClasses, nil
	}
	return curseforge.ResolveClasses(classesFlag)
}
