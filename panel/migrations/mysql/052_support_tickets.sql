-- Migration 052: Support ticket system — tickets, messages, attachments, canned responses

-- Support tickets
CREATE TABLE IF NOT EXISTS tickets (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NOT NULL,
  subject VARCHAR(255) NOT NULL,
  category ENUM('billing','technical','general') DEFAULT 'general',
  priority ENUM('low','medium','high') DEFAULT 'medium',
  status ENUM('open','in_progress','waiting','resolved','closed') DEFAULT 'open',
  assigned_to VARCHAR(64) NULL,
  satisfaction_rating TINYINT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  resolved_at TIMESTAMP NULL,
  INDEX idx_status (status),
  INDEX idx_customer (customer_id),
  FOREIGN KEY (customer_id) REFERENCES customers(id)
);

-- Conversation thread messages
CREATE TABLE IF NOT EXISTS ticket_messages (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  ticket_id BIGINT NOT NULL,
  sender_type ENUM('customer','admin') NOT NULL,
  sender_id VARCHAR(64) NOT NULL,
  body TEXT NOT NULL,
  is_internal BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE CASCADE
);

-- File attachments on messages
CREATE TABLE IF NOT EXISTS ticket_attachments (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  message_id BIGINT NOT NULL,
  filename VARCHAR(255) NOT NULL,
  filepath VARCHAR(512) NOT NULL,
  filesize INT NOT NULL,
  mime_type VARCHAR(100),
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (message_id) REFERENCES ticket_messages(id) ON DELETE CASCADE
);

-- Pre-written reply templates
CREATE TABLE IF NOT EXISTS canned_responses (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  title VARCHAR(255) NOT NULL,
  body TEXT NOT NULL,
  category VARCHAR(64) NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
