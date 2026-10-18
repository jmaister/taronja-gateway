CREATE TABLE IF NOT EXISTS `user_phones` (`user_id` varchar(255) NOT NULL,`phone` varchar(20) NOT NULL,`verified_at` datetime,`updated_at` datetime,PRIMARY KEY (`user_id`));
CREATE INDEX IF NOT EXISTS `idx_user_phones_phone` ON `user_phones`(`phone`);

CREATE TABLE IF NOT EXISTS `notification_link_tokens` (`token_hash` varchar(64) NOT NULL,`notification_id` varchar(255) NOT NULL,`channel` varchar(50) NOT NULL,`expires_at` datetime NOT NULL,`created_at` datetime,PRIMARY KEY (`token_hash`));
CREATE INDEX IF NOT EXISTS `idx_notification_link_tokens_notification_id` ON `notification_link_tokens`(`notification_id`);
