UPDATE job_queue
SET status = 'pending', attempts = ?, updated_at = ?
WHERE id = ?
