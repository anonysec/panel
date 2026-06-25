-- Node task queue. Panel creates tasks; node agents poll and complete them over HTTP.
CREATE TABLE IF NOT EXISTS node_tasks (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  node_id BIGINT NOT NULL,
  action VARCHAR(80) NOT NULL,
  payload_json JSON NULL,
  status ENUM('pending','running','succeeded','failed','cancelled') NOT NULL DEFAULT 'pending',
  result_json JSON NULL,
  error TEXT NULL,
  created_by VARCHAR(64) DEFAULT '',
  claimed_at DATETIME NULL,
  completed_at DATETIME NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX(node_id), INDEX(status), INDEX(action), INDEX(created_at)
);
