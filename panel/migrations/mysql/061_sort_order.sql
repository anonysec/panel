-- Add sort_order column to nodes table for drag-and-drop reordering.
-- plans table already has sort_order from 001_init.sql.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;
