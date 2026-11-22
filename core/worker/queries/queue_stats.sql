SELECT
	COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
	COALESCE(MIN(CASE WHEN status = 'pending' THEN created_at END), 0),
	(SELECT COUNT(*) FROM job_graveyard),
	(SELECT COUNT(*) FROM job_history)
FROM job_queue;
