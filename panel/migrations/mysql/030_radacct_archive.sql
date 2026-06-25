-- Migration 030: Radacct archive table for closed sessions older than 90 days
-- Keeps radacct lean for fast queries while preserving history

CREATE TABLE IF NOT EXISTS radacct_archive LIKE radacct;

-- Add archive date tracking
ALTER TABLE radacct_archive ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE radacct_archive DROP PRIMARY KEY, ADD PRIMARY KEY (radacctid);
