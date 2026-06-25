-- Reseller custom plan pricing
CREATE TABLE IF NOT EXISTS reseller_plan_prices (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  reseller_id BIGINT NOT NULL,
  plan_id BIGINT NOT NULL,
  sell_price DECIMAL(12,2) NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_reseller_plan (reseller_id, plan_id),
  INDEX(reseller_id),
  INDEX(plan_id)
);

-- Reseller tickets
CREATE TABLE IF NOT EXISTS reseller_tickets (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  reseller_username VARCHAR(64) NOT NULL,
  subject VARCHAR(255) NOT NULL,
  status ENUM('open','closed') NOT NULL DEFAULT 'open',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX(reseller_username),
  INDEX(status)
);

CREATE TABLE IF NOT EXISTS reseller_ticket_messages (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  ticket_id BIGINT NOT NULL,
  sender VARCHAR(64) NOT NULL,
  message TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(ticket_id)
);
