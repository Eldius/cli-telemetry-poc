CREATE TABLE IF NOT EXISTS mods (
    id                     INTEGER PRIMARY KEY,
    game_id                INTEGER NOT NULL,
    class_id               INTEGER,
    name                   TEXT    NOT NULL,
    slug                   TEXT,
    summary                TEXT,
    status                 INTEGER,
    download_count         INTEGER,
    is_featured            BOOLEAN,
    primary_category_id    INTEGER,
    website_url            TEXT,
    source_url             TEXT,
    issues_url             TEXT,
    wiki_url               TEXT,
    logo_url               TEXT,
    logo_thumbnail_url     TEXT,
    main_file_id           INTEGER,
    date_created           TIMESTAMP,
    date_modified          TIMESTAMP,
    date_released          TIMESTAMP,
    allow_mod_distribution BOOLEAN,
    game_popularity_rank   INTEGER,
    is_available           BOOLEAN,
    thumbs_up_count        INTEGER,
    categories_json        TEXT,
    authors_json           TEXT,
    latest_files_json      TEXT,
    latest_files_indexes_json TEXT,
    raw_json               TEXT    NOT NULL,
    synced_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_mods_class_id ON mods (class_id);
CREATE INDEX IF NOT EXISTS idx_mods_slug ON mods (slug);
CREATE INDEX IF NOT EXISTS idx_mods_date_modified ON mods (date_modified);
CREATE INDEX IF NOT EXISTS idx_mods_download_count ON mods (download_count);

CREATE TABLE IF NOT EXISTS categories (
    id                 INTEGER PRIMARY KEY,
    game_id            INTEGER NOT NULL,
    class_id           INTEGER,
    parent_category_id INTEGER,
    name               TEXT NOT NULL,
    slug               TEXT,
    url                TEXT,
    icon_url           TEXT,
    is_class           BOOLEAN,
    date_modified      TIMESTAMP,
    raw_json           TEXT NOT NULL,
    synced_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_categories_class_id ON categories (class_id);

CREATE TABLE IF NOT EXISTS sync_state (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
