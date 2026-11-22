DELETE FROM sessions
WHERE user_id = ?
RETURNING token_hash
