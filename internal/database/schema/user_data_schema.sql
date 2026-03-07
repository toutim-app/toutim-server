CREATE TABLE IF NOT EXISTS collections (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
    description TEXT,
    icon        TEXT,
    color       TEXT NOT NULL CHECK(length(color) BETWEEN 1 AND 20),
    template_id TEXT,
    view_mode   TEXT NOT NULL DEFAULT 'list',
    sync_status TEXT NOT NULL DEFAULT 'synced',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS items (
    id            TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    favorite      INTEGER NOT NULL DEFAULT 0,
    sync_status   TEXT NOT NULL DEFAULT 'synced',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS field_definitions (
    id            TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    name          TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
    type          TEXT NOT NULL,
    sort_order    INTEGER NOT NULL,
    is_required   INTEGER NOT NULL DEFAULT 0,
    show_in_list  INTEGER NOT NULL DEFAULT 1,
    show_in_card  INTEGER NOT NULL DEFAULT 1,
    searchable    INTEGER NOT NULL DEFAULT 1,
    sortable      INTEGER NOT NULL DEFAULT 0,
    default_value TEXT,
    options       TEXT NOT NULL DEFAULT '{}',
    sync_status   TEXT NOT NULL DEFAULT 'synced'
);

CREATE TABLE IF NOT EXISTS item_field_values (
    id                  TEXT PRIMARY KEY,
    item_id             TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    field_definition_id TEXT NOT NULL REFERENCES field_definitions(id) ON DELETE CASCADE,
    value               TEXT,
    updated_at          TEXT NOT NULL,
    UNIQUE(item_id, field_definition_id)
);

CREATE TABLE IF NOT EXISTS sync_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    entity     TEXT NOT NULL,
    entity_id  TEXT NOT NULL,
    action     TEXT NOT NULL,
    timestamp  TEXT NOT NULL,
    version    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sync_log_version ON sync_log(version);
CREATE INDEX IF NOT EXISTS idx_sync_log_entity ON sync_log(entity, entity_id);
CREATE INDEX IF NOT EXISTS idx_items_collection ON items(collection_id);
CREATE INDEX IF NOT EXISTS idx_field_defs_collection ON field_definitions(collection_id);
CREATE INDEX IF NOT EXISTS idx_field_values_item ON item_field_values(item_id);
