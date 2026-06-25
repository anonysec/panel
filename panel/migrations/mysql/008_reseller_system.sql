ALTER TABLE admins MODIFY COLUMN role ENUM('owner','admin','support','reseller') NOT NULL DEFAULT 'admin';
ALTER TABLE admins ADD COLUMN credit DECIMAL(12,2) NOT NULL DEFAULT 0.00;
