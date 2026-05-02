UPDATE job_queue
SET status = 'pending', updated_at = ?
WHERE status = 'running'
