CREATE TABLE IF NOT EXISTS reseller_allowed_plans (
  reseller_id BIGINT NOT NULL,
  plan_id BIGINT NOT NULL,
  PRIMARY KEY (reseller_id, plan_id),
  INDEX(reseller_id)
);
