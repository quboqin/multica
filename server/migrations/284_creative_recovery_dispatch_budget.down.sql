ALTER TABLE creative_recovery DROP CONSTRAINT creative_recovery_dispatch_limit;
ALTER TABLE creative_recovery DROP COLUMN dispatch_count;
ALTER TABLE creative_recovery DROP COLUMN dispatch_failures;
-- Preserve audit sequence numbers created after the upgrade.
