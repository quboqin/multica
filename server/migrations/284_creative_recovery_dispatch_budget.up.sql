-- Keep the original audit sequence, but charge only successful dispatches.
ALTER TABLE creative_recovery ADD COLUMN dispatch_count INT NOT NULL DEFAULT 0 CHECK(dispatch_count >= 0);
ALTER TABLE creative_recovery ADD COLUMN dispatch_failures INT NOT NULL DEFAULT 0 CHECK(dispatch_failures >= 0);
DO $$ DECLARE constraint_name TEXT; BEGIN
  SELECT conname INTO constraint_name FROM pg_constraint
  WHERE conrelid='creative_recovery'::regclass AND contype='c'
    AND pg_get_constraintdef(oid)='CHECK ((attempt <= max_attempts))';
  IF constraint_name IS NOT NULL THEN
    EXECUTE format('ALTER TABLE creative_recovery DROP CONSTRAINT %I',constraint_name);
  END IF;
END $$;
UPDATE creative_recovery r SET dispatch_count=LEAST(r.max_attempts, (
  SELECT count(*) FROM creative_recovery_attempt a WHERE a.recovery_id=r.id
  AND (a.result_task_id IS NOT NULL OR (r.stage='prime' AND a.status IN ('queued','resolved')))
));
ALTER TABLE creative_recovery ADD CONSTRAINT creative_recovery_dispatch_limit CHECK(dispatch_count <= max_attempts);
-- Re-evaluate only the records stranded by rejected dispatches. Do not reset
-- actual dispatches, visual rejections, cancellations or provider uncertainty.
UPDATE creative_recovery SET status='pending',next_retry_at=now(),updated_at=now()
WHERE status='manual_required' AND dispatch_count<max_attempts
AND last_error IN ('recovery retry budget or eligibility: no rows in result set',
                   'candidate selection is not eligible for another dispatch');
