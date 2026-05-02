UPDATE job_queue
SET status = 'running', updated_at = ?
WHERE status = 'pending'
RETURNING id, name, payload, attempts
