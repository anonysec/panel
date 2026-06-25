-- Migration 003: Default settings for alert thresholds and gRPC params
-- The panel_settings table already exists from 001_init.sql.
-- This migration seeds default values for the UI overhaul features.

INSERT INTO panel_settings (setting_key, setting_value) VALUES
  ('alert_cpu_threshold', '90'),
  ('alert_ram_threshold', '90'),
  ('alert_disk_threshold', '85'),
  ('grpc_connect_timeout', '10'),
  ('grpc_keepalive_interval', '30'),
  ('grpc_metrics_interval', '60')
ON CONFLICT (setting_key) DO NOTHING;
