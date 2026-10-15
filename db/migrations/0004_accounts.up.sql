CREATE TABLE IF NOT EXISTS `auth_tokens` (`id` varchar(255) NOT NULL,`purpose` varchar(30) NOT NULL,`user_id` varchar(255),`email` varchar(255) NOT NULL,`token_hash` varchar(64) NOT NULL,`code_hash` varchar(64),`binding_hash` varchar(64),`password_hash` varchar(255),`redirect` varchar(500),`attempts` integer NOT NULL DEFAULT 0,`expires_at` datetime,`used_at` datetime,`created_at` datetime,PRIMARY KEY (`id`));
CREATE INDEX IF NOT EXISTS `idx_auth_tokens_binding_hash` ON `auth_tokens`(`binding_hash`);
CREATE UNIQUE INDEX IF NOT EXISTS `idx_auth_tokens_token_hash` ON `auth_tokens`(`token_hash`);
CREATE INDEX IF NOT EXISTS `idx_auth_tokens_user_id` ON `auth_tokens`(`user_id`);
CREATE INDEX IF NOT EXISTS `idx_auth_tokens_purpose_email` ON `auth_tokens`(`purpose`,`email`);

CREATE TABLE IF NOT EXISTS `user_identities` (`id` varchar(255) NOT NULL,`user_id` varchar(255) NOT NULL,`provider` varchar(50) NOT NULL,`subject` varchar(255) NOT NULL,`email` varchar(255),`created_at` datetime,`last_login_at` datetime,PRIMARY KEY (`id`));
CREATE UNIQUE INDEX IF NOT EXISTS `idx_user_identities_provider_subject` ON `user_identities`(`provider`,`subject`);
CREATE INDEX IF NOT EXISTS `idx_user_identities_user_id` ON `user_identities`(`user_id`);

CREATE TABLE IF NOT EXISTS `auth_events` (`id` integer PRIMARY KEY AUTOINCREMENT,`user_id` varchar(255) NOT NULL,`type` varchar(50) NOT NULL,`provider` varchar(50),`ip_address` varchar(45),`user_agent` text,`created_at` datetime);
CREATE INDEX IF NOT EXISTS `idx_auth_events_created_at` ON `auth_events`(`created_at`);
CREATE INDEX IF NOT EXISTS `idx_auth_events_user_id` ON `auth_events`(`user_id`);

-- Users who signed in with an OAuth provider before identities existed get
-- their one identity, so they keep logging in the same way.
INSERT OR IGNORE INTO `user_identities` (`id`, `user_id`, `provider`, `subject`, `email`, `created_at`)
SELECT lower(hex(randomblob(16))), `id`, `provider`, `provider_id`, `email`, `created_at`
FROM `users`
WHERE `deleted_at` IS NULL
  AND `provider` IN ('google', 'github')
  AND `provider_id` IS NOT NULL AND `provider_id` != '';
