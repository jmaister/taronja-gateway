CREATE TABLE IF NOT EXISTS `fingerprint_first_seen` (
  `fingerprint` varchar(100) PRIMARY KEY,
  `first_timestamp` datetime NOT NULL
);

INSERT OR IGNORE INTO `fingerprint_first_seen` (`fingerprint`, `first_timestamp`)
SELECT `fingerprint`, MIN(`timestamp`)
FROM `traffic_metrics`
WHERE `fingerprint` IS NOT NULL AND `fingerprint` != ''
GROUP BY `fingerprint`;

CREATE INDEX IF NOT EXISTS `idx_fingerprint_first_seen_first_timestamp` ON `fingerprint_first_seen`(`first_timestamp`);

CREATE TRIGGER IF NOT EXISTS `trg_traffic_metrics_first_seen`
AFTER INSERT ON `traffic_metrics`
WHEN NEW.`fingerprint` IS NOT NULL AND NEW.`fingerprint` != ''
BEGIN
  INSERT INTO `fingerprint_first_seen` (`fingerprint`, `first_timestamp`)
  VALUES (NEW.`fingerprint`, NEW.`timestamp`)
  ON CONFLICT(`fingerprint`) DO UPDATE SET `first_timestamp` = MIN(`first_timestamp`, excluded.`first_timestamp`);
END;
