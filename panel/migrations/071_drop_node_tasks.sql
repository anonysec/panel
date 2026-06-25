-- Remove legacy node_tasks table (replaced by direct gRPC calls to knode).
-- First cancel any pending/running/claimed tasks, then drop the table.
UPDATE node_tasks SET status = 'cancelled', updated_at = NOW() WHERE status IN ('pending', 'running', 'claimed');
DROP TABLE IF EXISTS node_tasks;
