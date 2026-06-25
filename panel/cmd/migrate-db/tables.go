package main

// Table definitions for MariaDB → PostgreSQL migration.
// Each function returns a tableSpec with the PostgreSQL CREATE TABLE DDL
// and the column list used for SELECT/INSERT.
//
// Type mappings applied:
//   INT AUTO_INCREMENT  → BIGSERIAL
//   BIGINT AUTO_INCREMENT → BIGSERIAL
//   TINYINT(1)          → BOOLEAN
//   DATETIME / TIMESTAMP → TIMESTAMPTZ
//   LONGTEXT / TEXT      → TEXT
//   DECIMAL(12,2)       → NUMERIC(12,2)
//   ENUM(...)           → VARCHAR(40)
//   JSON                → JSONB
//   VARCHAR(N)          → VARCHAR(N)

func tableAdmins() tableSpec {
	return tableSpec{
		name:       "admins",
		primaryKey: "id",
		columns: []string{
			"id", "username", "password_hash", "role", "is_active",
			"credit", "created_at", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS admins (
			id BIGSERIAL PRIMARY KEY,
			username VARCHAR(64) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			role VARCHAR(40) NOT NULL DEFAULT 'admin',
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			credit NUMERIC(12,2) NOT NULL DEFAULT 0.00,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableAdminLoginAttempts() tableSpec {
	return tableSpec{
		name:       "admin_login_attempts",
		primaryKey: "id",
		columns: []string{
			"id", "ip", "username", "success", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS admin_login_attempts (
			id BIGSERIAL PRIMARY KEY,
			ip VARCHAR(64) NOT NULL,
			username VARCHAR(64),
			success BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_admin_login_ip ON admin_login_attempts(ip);
		CREATE INDEX IF NOT EXISTS idx_admin_login_user ON admin_login_attempts(username)`,
	}
}

func tablePlans() tableSpec {
	return tableSpec{
		name:       "plans",
		primaryKey: "id",
		columns: []string{
			"id", "name", "data_gb", "duration_days", "price",
			"is_active", "sort_order", "created_at", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS plans (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			data_gb NUMERIC(12,2) NOT NULL DEFAULT 0,
			duration_days INT NOT NULL DEFAULT 30,
			price NUMERIC(12,2) NOT NULL DEFAULT 0,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			sort_order INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableCustomers() tableSpec {
	return tableSpec{
		name:       "customers",
		primaryKey: "id",
		columns: []string{
			"id", "username", "display_name", "created_by", "plan_id",
			"status", "sub_token", "notes", "created_at", "updated_at", "deleted_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS customers (
			id BIGSERIAL PRIMARY KEY,
			username VARCHAR(64) NOT NULL UNIQUE,
			display_name VARCHAR(128),
			created_by VARCHAR(64),
			plan_id BIGINT,
			status VARCHAR(40) NOT NULL DEFAULT 'active',
			sub_token VARCHAR(96) UNIQUE,
			notes TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_customers_created_by ON customers(created_by);
		CREATE INDEX IF NOT EXISTS idx_customers_plan_id ON customers(plan_id);
		CREATE INDEX IF NOT EXISTS idx_customers_status ON customers(status)`,
	}
}

func tableDiscountCodes() tableSpec {
	return tableSpec{
		name:       "discount_codes",
		primaryKey: "code",
		columns: []string{
			"code", "percent", "amount", "max_uses", "used",
			"expires_at", "is_active", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS discount_codes (
			code VARCHAR(64) PRIMARY KEY,
			percent INT NOT NULL DEFAULT 0,
			amount NUMERIC(12,2) NOT NULL DEFAULT 0,
			max_uses INT NOT NULL DEFAULT 0,
			used INT NOT NULL DEFAULT 0,
			expires_at TIMESTAMPTZ,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableSubscriptions() tableSpec {
	return tableSpec{
		name:       "subscriptions",
		primaryKey: "id",
		columns: []string{
			"id", "customer_id", "username", "plan_id", "status",
			"started_at", "expires_at", "paid_amount", "discount_code",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS subscriptions (
			id BIGSERIAL PRIMARY KEY,
			customer_id BIGINT,
			username VARCHAR(64) NOT NULL,
			plan_id BIGINT,
			status VARCHAR(40) NOT NULL DEFAULT 'active',
			started_at TIMESTAMPTZ DEFAULT NOW(),
			expires_at TIMESTAMPTZ,
			paid_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
			discount_code VARCHAR(64)
		);
		CREATE INDEX IF NOT EXISTS idx_subs_customer ON subscriptions(customer_id);
		CREATE INDEX IF NOT EXISTS idx_subs_username ON subscriptions(username);
		CREATE INDEX IF NOT EXISTS idx_subs_plan ON subscriptions(plan_id);
		CREATE INDEX IF NOT EXISTS idx_subs_status ON subscriptions(status)`,
	}
}

func tableWallets() tableSpec {
	return tableSpec{
		name:       "wallets",
		primaryKey: "username",
		columns: []string{
			"customer_id", "username", "credit", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS wallets (
			customer_id BIGINT,
			username VARCHAR(64) PRIMARY KEY,
			credit NUMERIC(12,2) NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_wallets_customer ON wallets(customer_id)`,
	}
}

func tableWalletTransactions() tableSpec {
	return tableSpec{
		name:       "wallet_transactions",
		primaryKey: "id",
		columns: []string{
			"id", "customer_id", "username", "amount", "type",
			"description", "actor", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS wallet_transactions (
			id BIGSERIAL PRIMARY KEY,
			customer_id BIGINT,
			username VARCHAR(64) NOT NULL,
			amount NUMERIC(12,2) NOT NULL,
			type VARCHAR(40) NOT NULL DEFAULT 'adjustment',
			description VARCHAR(255) DEFAULT '',
			actor VARCHAR(64) DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_wtx_customer ON wallet_transactions(customer_id);
		CREATE INDEX IF NOT EXISTS idx_wtx_username ON wallet_transactions(username);
		CREATE INDEX IF NOT EXISTS idx_wtx_type ON wallet_transactions(type)`,
	}
}

func tablePaymentMethods() tableSpec {
	return tableSpec{
		name:       "payment_methods",
		primaryKey: "id",
		columns: []string{
			"id", "name", "type", "config_json", "is_active", "sort_order", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS payment_methods (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(80) NOT NULL,
			type VARCHAR(40) NOT NULL DEFAULT 'manual',
			config_json JSONB,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			sort_order INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tablePayments() tableSpec {
	return tableSpec{
		name:       "payments",
		primaryKey: "id",
		columns: []string{
			"id", "customer_id", "username", "amount", "method",
			"receipt", "receipt_file", "status", "admin_note",
			"created_at", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS payments (
			id BIGSERIAL PRIMARY KEY,
			customer_id BIGINT,
			username VARCHAR(64) NOT NULL,
			amount NUMERIC(12,2) NOT NULL,
			method VARCHAR(64) DEFAULT 'manual',
			receipt TEXT,
			receipt_file VARCHAR(255),
			status VARCHAR(40) NOT NULL DEFAULT 'pending',
			admin_note TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_payments_customer ON payments(customer_id);
		CREATE INDEX IF NOT EXISTS idx_payments_username ON payments(username);
		CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status)`,
	}
}

func tableTickets() tableSpec {
	return tableSpec{
		name:       "tickets",
		primaryKey: "id",
		columns: []string{
			"id", "customer_id", "username", "subject", "status",
			"priority", "created_at", "updated_at", "closed_at", "deleted_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS tickets (
			id BIGSERIAL PRIMARY KEY,
			customer_id BIGINT,
			username VARCHAR(64) NOT NULL,
			subject VARCHAR(160) NOT NULL,
			status VARCHAR(40) NOT NULL DEFAULT 'open',
			priority VARCHAR(40) NOT NULL DEFAULT 'normal',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			closed_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_tickets_customer ON tickets(customer_id);
		CREATE INDEX IF NOT EXISTS idx_tickets_username ON tickets(username);
		CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status)`,
	}
}

func tableTicketMessages() tableSpec {
	return tableSpec{
		name:       "ticket_messages",
		primaryKey: "id",
		columns: []string{
			"id", "ticket_id", "sender_type", "sender_name", "message", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS ticket_messages (
			id BIGSERIAL PRIMARY KEY,
			ticket_id BIGINT NOT NULL,
			sender_type VARCHAR(40) NOT NULL,
			sender_name VARCHAR(64) NOT NULL,
			message TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_tmsg_ticket ON ticket_messages(ticket_id);
		CREATE INDEX IF NOT EXISTS idx_tmsg_sender ON ticket_messages(sender_type)`,
	}
}

func tableNodes() tableSpec {
	return tableSpec{
		name:       "nodes",
		primaryKey: "id",
		columns: []string{
			"id", "name", "public_ip", "domain", "api_token_hash",
			"status", "last_seen_at", "created_at", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS nodes (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(64) NOT NULL UNIQUE,
			public_ip VARCHAR(64) NOT NULL,
			domain VARCHAR(255),
			api_token_hash VARCHAR(128) NOT NULL,
			status VARCHAR(40) NOT NULL DEFAULT 'offline',
			last_seen_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableNodeStatus() tableSpec {
	return tableSpec{
		name:       "node_status",
		primaryKey: "node_id",
		columns: []string{
			"node_id", "cpu_percent", "ram_percent", "disk_percent",
			"rx_bps", "tx_bps", "openvpn_status", "l2tp_status",
			"ikev2_status", "payload_json", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS node_status (
			node_id BIGINT PRIMARY KEY,
			cpu_percent NUMERIC(6,2) DEFAULT 0,
			ram_percent NUMERIC(6,2) DEFAULT 0,
			disk_percent NUMERIC(6,2) DEFAULT 0,
			rx_bps BIGINT DEFAULT 0,
			tx_bps BIGINT DEFAULT 0,
			openvpn_status VARCHAR(24) DEFAULT 'unknown',
			l2tp_status VARCHAR(24) DEFAULT 'unknown',
			ikev2_status VARCHAR(24) DEFAULT 'unknown',
			payload_json JSONB,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableNodeServices() tableSpec {
	return tableSpec{
		name:       "node_services",
		primaryKey: "id",
		columns: []string{
			"id", "node_id", "service", "status", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS node_services (
			id BIGSERIAL PRIMARY KEY,
			node_id BIGINT NOT NULL,
			service VARCHAR(40) NOT NULL,
			status VARCHAR(24) NOT NULL DEFAULT 'unknown',
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(node_id, service)
		)`,
	}
}

func tableNodeUsageSnapshots() tableSpec {
	return tableSpec{
		name:       "node_usage_snapshots",
		primaryKey: "id",
		columns: []string{
			"id", "node_id", "rx_bytes", "tx_bytes", "online_users", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS node_usage_snapshots (
			id BIGSERIAL PRIMARY KEY,
			node_id BIGINT NOT NULL,
			rx_bytes BIGINT NOT NULL DEFAULT 0,
			tx_bytes BIGINT NOT NULL DEFAULT 0,
			online_users INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_nusage_node ON node_usage_snapshots(node_id);
		CREATE INDEX IF NOT EXISTS idx_nusage_time ON node_usage_snapshots(created_at)`,
	}
}

func tableNodeTasks() tableSpec {
	return tableSpec{
		name:       "node_tasks",
		primaryKey: "id",
		columns: []string{
			"id", "node_id", "action", "payload_json", "status",
			"result_json", "error", "created_by", "claimed_at",
			"completed_at", "created_at", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS node_tasks (
			id BIGSERIAL PRIMARY KEY,
			node_id BIGINT NOT NULL,
			action VARCHAR(80) NOT NULL,
			payload_json JSONB,
			status VARCHAR(40) NOT NULL DEFAULT 'pending',
			result_json JSONB,
			error TEXT,
			created_by VARCHAR(64) DEFAULT '',
			claimed_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_ntask_node ON node_tasks(node_id);
		CREATE INDEX IF NOT EXISTS idx_ntask_status ON node_tasks(status);
		CREATE INDEX IF NOT EXISTS idx_ntask_action ON node_tasks(action);
		CREATE INDEX IF NOT EXISTS idx_ntask_created ON node_tasks(created_at)`,
	}
}

func tableVpnCoreSettings() tableSpec {
	return tableSpec{
		name:       "vpn_core_settings",
		primaryKey: "id",
		columns: []string{
			"id", "openvpn_port", "openvpn_protocol", "openvpn_network",
			"l2tp_network", "ikev2_network", "ipsec_psk",
			"dns_1", "dns_2", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS vpn_core_settings (
			id SMALLINT PRIMARY KEY DEFAULT 1,
			openvpn_port INT NOT NULL DEFAULT 1194,
			openvpn_protocol VARCHAR(10) NOT NULL DEFAULT 'udp',
			openvpn_network VARCHAR(32) NOT NULL DEFAULT '10.8.0.0/24',
			l2tp_network VARCHAR(32) NOT NULL DEFAULT '10.9.0.0/24',
			ikev2_network VARCHAR(32) NOT NULL DEFAULT '10.10.0.0/24',
			ipsec_psk VARCHAR(128),
			dns_1 VARCHAR(64) NOT NULL DEFAULT '1.1.1.1',
			dns_2 VARCHAR(64) NOT NULL DEFAULT '8.8.8.8',
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableVpnProfiles() tableSpec {
	return tableSpec{
		name:       "vpn_profiles",
		primaryKey: "id",
		columns: []string{
			"id", "type", "name", "file_path", "version", "is_active", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS vpn_profiles (
			id BIGSERIAL PRIMARY KEY,
			type VARCHAR(40) NOT NULL,
			name VARCHAR(80) NOT NULL,
			file_path VARCHAR(255),
			version INT NOT NULL DEFAULT 1,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_vpnprofiles_type ON vpn_profiles(type);
		CREATE INDEX IF NOT EXISTS idx_vpnprofiles_active ON vpn_profiles(is_active)`,
	}
}

func tableWgPeers() tableSpec {
	return tableSpec{
		name:       "wg_peers",
		primaryKey: "id",
		columns: []string{
			"id", "customer_id", "node_id", "public_key", "preshared_key",
			"private_key_encrypted", "allowed_ips", "endpoint", "status",
			"last_handshake_at", "rx_bytes", "tx_bytes", "created_at", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS wg_peers (
			id BIGSERIAL PRIMARY KEY,
			customer_id BIGINT,
			node_id BIGINT NOT NULL,
			public_key VARCHAR(44) NOT NULL,
			preshared_key VARCHAR(44),
			private_key_encrypted TEXT,
			allowed_ips VARCHAR(128) NOT NULL,
			endpoint VARCHAR(128),
			status VARCHAR(40) NOT NULL DEFAULT 'active',
			last_handshake_at TIMESTAMPTZ,
			rx_bytes BIGINT NOT NULL DEFAULT 0,
			tx_bytes BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(node_id, public_key)
		);
		CREATE INDEX IF NOT EXISTS idx_wgpeers_customer ON wg_peers(customer_id);
		CREATE INDEX IF NOT EXISTS idx_wgpeers_node ON wg_peers(node_id);
		CREATE INDEX IF NOT EXISTS idx_wgpeers_status ON wg_peers(status)`,
	}
}

func tableApiKeys() tableSpec {
	return tableSpec{
		name:       "api_keys",
		primaryKey: "id",
		columns: []string{
			"id", "name", "key_hash", "scopes", "enabled", "last4",
			"created_at", "last_used_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS api_keys (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(80) NOT NULL UNIQUE,
			key_hash VARCHAR(128) NOT NULL,
			scopes TEXT,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			last4 VARCHAR(8),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			last_used_at TIMESTAMPTZ
		)`,
	}
}

func tableApiLogs() tableSpec {
	return tableSpec{
		name:       "api_logs",
		primaryKey: "id",
		columns: []string{
			"id", "key_name", "action", "ip", "success", "message", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS api_logs (
			id BIGSERIAL PRIMARY KEY,
			key_name VARCHAR(80),
			action VARCHAR(80),
			ip VARCHAR(64),
			success BOOLEAN NOT NULL DEFAULT FALSE,
			message TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_apilogs_key ON api_logs(key_name);
		CREATE INDEX IF NOT EXISTS idx_apilogs_action ON api_logs(action);
		CREATE INDEX IF NOT EXISTS idx_apilogs_time ON api_logs(created_at)`,
	}
}

func tableEvents() tableSpec {
	return tableSpec{
		name:       "events",
		primaryKey: "id",
		columns: []string{
			"id", "type", "severity", "title", "message",
			"actor", "related", "seen", "notified", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS events (
			id BIGSERIAL PRIMARY KEY,
			type VARCHAR(40) NOT NULL,
			severity VARCHAR(20) NOT NULL DEFAULT 'info',
			title VARCHAR(160) NOT NULL,
			message TEXT,
			actor VARCHAR(64) DEFAULT '',
			related VARCHAR(128) DEFAULT '',
			seen BOOLEAN NOT NULL DEFAULT FALSE,
			notified BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
		CREATE INDEX IF NOT EXISTS idx_events_severity ON events(severity);
		CREATE INDEX IF NOT EXISTS idx_events_seen ON events(seen);
		CREATE INDEX IF NOT EXISTS idx_events_time ON events(created_at)`,
	}
}

func tableAuditLogs() tableSpec {
	return tableSpec{
		name:       "audit_logs",
		primaryKey: "id",
		columns: []string{
			"id", "actor", "action", "entity_type", "entity_id",
			"before_json", "after_json", "ip", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGSERIAL PRIMARY KEY,
			actor VARCHAR(64) NOT NULL,
			action VARCHAR(80) NOT NULL,
			entity_type VARCHAR(40) NOT NULL,
			entity_id VARCHAR(80),
			before_json JSONB,
			after_json JSONB,
			ip VARCHAR(64),
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor);
		CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action);
		CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_logs(entity_type);
		CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_logs(created_at)`,
	}
}

func tableDeletedArchive() tableSpec {
	return tableSpec{
		name:       "deleted_archive",
		primaryKey: "id",
		columns: []string{
			"id", "type", "name", "archive_key", "payload",
			"created_by", "created_at", "restored_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS deleted_archive (
			id BIGSERIAL PRIMARY KEY,
			type VARCHAR(32) NOT NULL,
			name VARCHAR(128) NOT NULL,
			archive_key VARCHAR(128),
			payload TEXT,
			created_by VARCHAR(64),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			restored_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_archive_type ON deleted_archive(type);
		CREATE INDEX IF NOT EXISTS idx_archive_name ON deleted_archive(name);
		CREATE INDEX IF NOT EXISTS idx_archive_time ON deleted_archive(created_at)`,
	}
}

func tableSettings() tableSpec {
	return tableSpec{
		name:       "settings",
		primaryKey: "name",
		columns: []string{
			"name", "value", "type", "group_name", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS settings (
			name VARCHAR(80) PRIMARY KEY,
			value TEXT,
			type VARCHAR(32) DEFAULT 'string',
			group_name VARCHAR(64) DEFAULT 'general',
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableFirewallRules() tableSpec {
	return tableSpec{
		name:       "firewall_rules",
		primaryKey: "id",
		columns: []string{
			"id", "node_id", "name", "type", "direction",
			"source", "destination", "protocol", "port",
			"action", "priority", "is_active", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS firewall_rules (
			id BIGSERIAL PRIMARY KEY,
			node_id BIGINT,
			name VARCHAR(100) NOT NULL,
			type VARCHAR(40) NOT NULL,
			direction VARCHAR(20) NOT NULL DEFAULT 'forward',
			source VARCHAR(200),
			destination VARCHAR(200),
			protocol VARCHAR(20),
			port VARCHAR(50),
			action VARCHAR(20) NOT NULL DEFAULT 'drop',
			priority INT NOT NULL DEFAULT 100,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_fw_node_active ON firewall_rules(node_id, is_active, priority)`,
	}
}

func tablePanelSettings() tableSpec {
	return tableSpec{
		name:       "panel_settings",
		primaryKey: "setting_key",
		columns: []string{
			"setting_key", "setting_value", "updated_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS panel_settings (
			setting_key VARCHAR(64) PRIMARY KEY,
			setting_value TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableBandwidthRules() tableSpec {
	return tableSpec{
		name:       "bandwidth_rules",
		primaryKey: "id",
		columns: []string{
			"id", "username", "download_kbps", "upload_kbps",
			"priority", "is_active", "created_at",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS bandwidth_rules (
			id BIGSERIAL PRIMARY KEY,
			username VARCHAR(64) NOT NULL UNIQUE,
			download_kbps INT NOT NULL DEFAULT 0,
			upload_kbps INT NOT NULL DEFAULT 0,
			priority VARCHAR(20) NOT NULL DEFAULT 'normal',
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
}

func tableRadcheck() tableSpec {
	return tableSpec{
		name:       "radcheck",
		primaryKey: "id",
		columns: []string{
			"id", "username", "attribute", "op", "value",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS radcheck (
			id BIGSERIAL PRIMARY KEY,
			username VARCHAR(64) NOT NULL DEFAULT '',
			attribute VARCHAR(64) NOT NULL DEFAULT '',
			op VARCHAR(2) NOT NULL DEFAULT ':=',
			value VARCHAR(253) NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_radcheck_username ON radcheck(username)`,
	}
}

func tableRadacct() tableSpec {
	return tableSpec{
		name:       "radacct",
		primaryKey: "radacctid",
		columns: []string{
			"radacctid", "acctsessionid", "acctuniqueid", "username",
			"realm", "nasipaddress", "nasportid", "nasporttype",
			"acctstarttime", "acctupdatetime", "acctstoptime",
			"acctinterval", "acctsessiontime", "acctauthentic",
			"connectinfo_start", "connectinfo_stop",
			"acctinputoctets", "acctoutputoctets",
			"calledstationid", "callingstationid",
			"acctterminatecause", "servicetype", "framedprotocol",
			"framedipaddress", "framedipv6address", "framedipv6prefix",
			"framedinterfaceid", "delegatedipv6prefix",
		},
		createDDL: `CREATE TABLE IF NOT EXISTS radacct (
			radacctid BIGSERIAL PRIMARY KEY,
			acctsessionid VARCHAR(64) NOT NULL DEFAULT '',
			acctuniqueid VARCHAR(32) NOT NULL DEFAULT '',
			username VARCHAR(64) NOT NULL DEFAULT '',
			realm VARCHAR(64) DEFAULT '',
			nasipaddress VARCHAR(15) NOT NULL DEFAULT '',
			nasportid VARCHAR(32) DEFAULT NULL,
			nasporttype VARCHAR(32) DEFAULT NULL,
			acctstarttime TIMESTAMPTZ,
			acctupdatetime TIMESTAMPTZ,
			acctstoptime TIMESTAMPTZ,
			acctinterval INT DEFAULT NULL,
			acctsessiontime INT DEFAULT NULL,
			acctauthentic VARCHAR(32) DEFAULT NULL,
			connectinfo_start VARCHAR(128) DEFAULT NULL,
			connectinfo_stop VARCHAR(128) DEFAULT NULL,
			acctinputoctets BIGINT DEFAULT 0,
			acctoutputoctets BIGINT DEFAULT 0,
			calledstationid VARCHAR(64) NOT NULL DEFAULT '',
			callingstationid VARCHAR(64) NOT NULL DEFAULT '',
			acctterminatecause VARCHAR(32) NOT NULL DEFAULT '',
			servicetype VARCHAR(32) DEFAULT NULL,
			framedprotocol VARCHAR(32) DEFAULT NULL,
			framedipaddress VARCHAR(15) NOT NULL DEFAULT '',
			framedipv6address VARCHAR(45) NOT NULL DEFAULT '',
			framedipv6prefix VARCHAR(45) NOT NULL DEFAULT '',
			framedinterfaceid VARCHAR(44) NOT NULL DEFAULT '',
			delegatedipv6prefix VARCHAR(45) NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_radacct_username ON radacct(username);
		CREATE INDEX IF NOT EXISTS idx_radacct_active ON radacct(username, acctstoptime);
		CREATE INDEX IF NOT EXISTS idx_radacct_start ON radacct(acctstarttime)`,
	}
}
