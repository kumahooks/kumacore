SELECT r.permissions
FROM users_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = ?
