-- Default no-package plan. It gives no fixed quota/speed/expiry and costs 0.
-- UI uses this as the default create-customer plan while fields stay blank/unlimited.
INSERT INTO plans(name,data_gb,speed_mbps,duration_days,price,is_active,sort_order)
SELECT 'Pay as you go',0,0,0,0,1,-100
WHERE NOT EXISTS (SELECT 1 FROM plans WHERE name='Pay as you go');
