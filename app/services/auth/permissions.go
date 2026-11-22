package authservice

const (
	// PermissionUnlimitedStorage grants unlimited storage capped only by available disk space.
	PermissionUnlimitedStorage int64 = 1 << 0
)

// PermissionEntry pairs a permission bit with its human-readable description. It is the exported view of the registry,
// used by surfaces that need to enumerate every known permission.
type PermissionEntry struct {
	Bit         int64
	Description string
}

var permissionRegistry = []PermissionEntry{
	{PermissionUnlimitedStorage, "unlimited storage"},
}

// PermissionEntries returns every registered permission bit with its description, in registry order. The returned slice
// is a copy of the registry, safe for callers to mutate.
func PermissionEntries() []PermissionEntry {
	entries := make([]PermissionEntry, len(permissionRegistry))
	copy(entries, permissionRegistry)

	return entries
}
