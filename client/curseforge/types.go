package curseforge

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GameIDMinecraft is the CurseForge game ID for Minecraft.
const GameIDMinecraft = 432

// Minecraft content class IDs on CurseForge.
const (
	ClassBukkitPlugins = 5
	ClassMods          = 6
	ClassResourcePacks = 12
	ClassWorlds        = 17
	ClassModpacks      = 4471
	ClassCustomization = 4546
	ClassAddons        = 4559
	ClassShaders       = 6552
	ClassDataPacks     = 6945
)

// ClassInfo describes a Minecraft content class on CurseForge.
type ClassInfo struct {
	ID   int
	Name string
	Slug string
}

// KnownClasses lists the Minecraft content classes known to this tool.
var KnownClasses = []ClassInfo{
	{ID: ClassMods, Name: "Mods", Slug: "mods"},
	{ID: ClassModpacks, Name: "Modpacks", Slug: "modpacks"},
	{ID: ClassAddons, Name: "Addons", Slug: "addons"},
	{ID: ClassResourcePacks, Name: "Resource Packs", Slug: "resource-packs"},
	{ID: ClassWorlds, Name: "Worlds", Slug: "worlds"},
	{ID: ClassBukkitPlugins, Name: "Bukkit Plugins", Slug: "bukkit-plugins"},
	{ID: ClassCustomization, Name: "Customization", Slug: "customization"},
	{ID: ClassShaders, Name: "Shaders", Slug: "shaders"},
	{ID: ClassDataPacks, Name: "Data Packs", Slug: "data-packs"},
}

// ClassName returns a human-readable name for a class ID.
func ClassName(id int) string {
	for _, c := range KnownClasses {
		if c.ID == id {
			return c.Name
		}
	}
	return fmt.Sprintf("class-%d", id)
}

// ResolveClasses maps class names, slugs or numeric IDs to ClassInfo values.
// Unknown names yield an error; unknown numeric IDs are accepted with a
// generated name so new CurseForge classes keep working.
func ResolveClasses(names []string) ([]ClassInfo, error) {
	byName := make(map[string]ClassInfo, len(KnownClasses)*2)
	for _, c := range KnownClasses {
		byName[strings.ToLower(c.Name)] = c
		byName[strings.ToLower(c.Slug)] = c
	}

	var out []ClassInfo
	for _, n := range names {
		key := strings.ToLower(strings.TrimSpace(n))
		if c, ok := byName[key]; ok {
			out = append(out, c)
			continue
		}
		if id, err := strconv.Atoi(key); err == nil {
			out = append(out, ClassInfo{ID: id, Name: ClassName(id), Slug: key})
			continue
		}
		return nil, fmt.Errorf("unknown class %q (known: %s)", n, KnownClassNames())
	}
	return out, nil
}

// KnownClassNames returns a comma-separated list of known class slugs.
func KnownClassNames() string {
	names := make([]string, 0, len(KnownClasses))
	for _, c := range KnownClasses {
		names = append(names, c.Slug)
	}
	return strings.Join(names, ", ")
}

// ClassUsesLoaders reports whether content of a class is associated with
// mod loaders (used to decide partition strategies).
func ClassUsesLoaders(classID int) bool {
	return classID == ClassMods || classID == ClassModpacks
}

// Mod search sort fields (CurseForge ModsSearchSortField).
const (
	SortFieldFeatured       = 1
	SortFieldPopularity     = 2
	SortFieldLastUpdated    = 3
	SortFieldName           = 4
	SortFieldAuthor         = 5
	SortFieldTotalDownloads = 6
	SortFieldCategory       = 7
	SortFieldGameVersion    = 8
)

// Sort orders.
const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

// ModLoaderType values accepted by the search API.
const (
	ModLoaderAny        = 0
	ModLoaderForge      = 1
	ModLoaderCauldron   = 2
	ModLoaderLiteLoader = 3
	ModLoaderFabric     = 4
	ModLoaderQuilt      = 5
	ModLoaderNeoForge   = 6
)

// LoaderIDs lists the mod loader filter values used when partitioning.
var LoaderIDs = []int{
	ModLoaderForge,
	ModLoaderCauldron,
	ModLoaderLiteLoader,
	ModLoaderFabric,
	ModLoaderQuilt,
	ModLoaderNeoForge,
}

// loaderNames maps CurseForge ModLoaderType IDs to their lowercase names.
var loaderNames = map[int]string{
	ModLoaderForge:      "forge",
	ModLoaderCauldron:   "cauldron",
	ModLoaderLiteLoader: "liteloader",
	ModLoaderFabric:     "fabric",
	ModLoaderQuilt:      "quilt",
	ModLoaderNeoForge:   "neoforge",
}

// LoaderName returns a human-readable name for a ModLoaderType ID.
func LoaderName(id int) string {
	if name, ok := loaderNames[id]; ok {
		return name
	}
	return fmt.Sprintf("loader-%d", id)
}

// KnownLoaderNames returns a comma-separated list of known loader names.
func KnownLoaderNames() string {
	names := make([]string, 0, len(LoaderIDs))
	for _, id := range LoaderIDs {
		names = append(names, loaderNames[id])
	}
	return strings.Join(names, ", ")
}

// ResolveLoader maps a mod loader name or numeric ID to its CurseForge
// ModLoaderType. "any" and "" resolve to ModLoaderAny.
func ResolveLoader(name string) (int, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" || key == "any" {
		return ModLoaderAny, nil
	}
	for id, loaderName := range loaderNames {
		if loaderName == key {
			return id, nil
		}
	}
	if id, err := strconv.Atoi(key); err == nil {
		return id, nil
	}
	return 0, fmt.Errorf("unknown mod loader %q (known: any, %s)", name, KnownLoaderNames())
}

// Pagination mirrors the API pagination object.
type Pagination struct {
	Index       int `json:"index"`
	PageSize    int `json:"pageSize"`
	ResultCount int `json:"resultCount"`
	TotalCount  int `json:"totalCount"`
}

// ModLinks holds the external links of a mod.
type ModLinks struct {
	WebsiteURL string `json:"websiteUrl"`
	WikiURL    string `json:"wikiUrl"`
	IssuesURL  string `json:"issuesUrl"`
	SourceURL  string `json:"sourceUrl"`
}

// Category mirrors the API category object.
type Category struct {
	ID               int       `json:"id"`
	GameID           int       `json:"gameId"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	URL              string    `json:"url"`
	IconURL          string    `json:"iconUrl"`
	DateModified     time.Time `json:"dateModified"`
	IsClass          bool      `json:"isClass"`
	ClassID          int       `json:"classId"`
	ParentCategoryID int       `json:"parentCategoryId"`
	DisplayIndex     int       `json:"displayIndex"`
}

// Author mirrors the API author object.
type Author struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ModAsset mirrors the API logo/screenshot object.
type ModAsset struct {
	ID           int    `json:"id"`
	ModID        int    `json:"modId"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	ThumbnailURL string `json:"thumbnailUrl"`
	URL          string `json:"url"`
}

// FileHash mirrors the API file hash object.
type FileHash struct {
	Value string `json:"value"`
	Algo  int    `json:"algo"`
}

// FileDependency mirrors the API file dependency object.
type FileDependency struct {
	ModID        int `json:"modId"`
	RelationType int `json:"relationType"`
}

// File mirrors the API file object (subset of fields).
type File struct {
	ID            int              `json:"id"`
	GameID        int              `json:"gameId"`
	ModID         int              `json:"modId"`
	IsAvailable   bool             `json:"isAvailable"`
	DisplayName   string           `json:"displayName"`
	FileName      string           `json:"fileName"`
	ReleaseType   int              `json:"releaseType"`
	FileStatus    int              `json:"fileStatus"`
	Hashes        []FileHash       `json:"hashes"`
	FileDate      time.Time        `json:"fileDate"`
	FileLength    int64            `json:"fileLength"`
	DownloadCount int64            `json:"downloadCount"`
	DownloadURL   *string          `json:"downloadUrl"`
	GameVersions  []string         `json:"gameVersions"`
	Dependencies  []FileDependency `json:"dependencies"`
}

// FileIndex mirrors the API file index object.
type FileIndex struct {
	GameVersion       string `json:"gameVersion"`
	FileID            int    `json:"fileId"`
	Filename          string `json:"filename"`
	ReleaseType       int    `json:"releaseType"`
	GameVersionTypeID int    `json:"gameVersionTypeId"`
	ModLoader         int    `json:"modLoader"`
}

// Mod mirrors the API mod object.
type Mod struct {
	ID                   int         `json:"id"`
	GameID               int         `json:"gameId"`
	Name                 string      `json:"name"`
	Slug                 string      `json:"slug"`
	Links                ModLinks    `json:"links"`
	Summary              string      `json:"summary"`
	Status               int         `json:"status"`
	DownloadCount        int64       `json:"downloadCount"`
	IsFeatured           bool        `json:"isFeatured"`
	PrimaryCategoryID    int         `json:"primaryCategoryId"`
	Categories           []Category  `json:"categories"`
	ClassID              int         `json:"classId"`
	Authors              []Author    `json:"authors"`
	Logo                 *ModAsset   `json:"logo"`
	Screenshots          []ModAsset  `json:"screenshots"`
	MainFileID           int         `json:"mainFileId"`
	LatestFiles          []File      `json:"latestFiles"`
	LatestFilesIndexes   []FileIndex `json:"latestFilesIndexes"`
	DateCreated          time.Time   `json:"dateCreated"`
	DateModified         time.Time   `json:"dateModified"`
	DateReleased         time.Time   `json:"dateReleased"`
	AllowModDistribution *bool       `json:"allowModDistribution"`
	GamePopularityRank   int         `json:"gamePopularityRank"`
	IsAvailable          bool        `json:"isAvailable"`
	ThumbsUpCount        int         `json:"thumbsUpCount"`
}

// ModEnvelope pairs a decoded mod with its raw JSON payload for lossless
// caching.
type ModEnvelope struct {
	Mod Mod
	Raw json.RawMessage
}

// SearchParams holds the filters for a mod search request.
type SearchParams struct {
	ClassID       int
	CategoryID    int
	GameVersion   string
	ModLoaderType int
	SortField     int
	SortOrder     string
	Index         int
	PageSize      int
}

// SearchResult is a page of mod search results.
type SearchResult struct {
	Mods       []ModEnvelope
	Pagination Pagination
}

// GameVersion mirrors the API game version object.
type GameVersion struct {
	ID                int       `json:"id"`
	GameID            int       `json:"gameId"`
	Name              string    `json:"name"`
	Slug              string    `json:"slug"`
	APIStatus         int       `json:"apiStatus"`
	DateModified      time.Time `json:"dateModified"`
	GameVersionTypeID int       `json:"gameVersionTypeId"`
}
