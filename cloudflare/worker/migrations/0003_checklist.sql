-- Checklist de entrada do aparelho + fotos da OS.

ALTER TABLE service_orders ADD COLUMN checklist_json TEXT NOT NULL DEFAULT '';
ALTER TABLE service_orders ADD COLUMN photos_json TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS checklist_templates (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    name TEXT NOT NULL,
    device_type TEXT NOT NULL DEFAULT '',
    items_json TEXT NOT NULL DEFAULT '[]',
    revision INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id),
    FOREIGN KEY (updated_by) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_checklist_templates_org ON checklist_templates(organization_id, updated_at DESC);
