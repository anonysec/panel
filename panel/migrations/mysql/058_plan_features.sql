-- Migration 058: Add features column to plans for landing page display
ALTER TABLE plans ADD COLUMN IF NOT EXISTS features TEXT DEFAULT NULL AFTER max_connections;
