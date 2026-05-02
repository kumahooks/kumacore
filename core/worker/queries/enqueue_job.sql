INSERT INTO job_queue (id, name, payload, attempts, status, created_at, updated_at)
VALUES (?, ?, ?, 0, 'pending', ?, ?)
