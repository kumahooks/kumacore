package authservice

import "testing"

// allPermissionConsts is the exhaustive list of exported permission bits. It mirrors the const block in permissions.go.
// The registry-coverage test uses it to catch the case where a new permission const is added here but omitted from
// permissionRegistry, which would make the bit invisible in the UI.
var allPermissionConsts = []int64{
	PermissionUnlimitedStorage,
}

// TestPermissionRegistry_CoversAllConsts asserts every exported permission const has a registered human-readable label,
// and that the registry holds no bits outside the known consts.
func TestPermissionRegistry_CoversAllConsts(t *testing.T) {
	var expectedMask int64
	for _, bit := range allPermissionConsts {
		expectedMask |= bit
	}

	var actualMask int64
	for _, entry := range permissionRegistry {
		if entry.Description == "" {
			t.Errorf("permission bit %d has empty description", entry.Bit)
		}

		actualMask |= entry.Bit
	}

	if expectedMask != actualMask {
		t.Errorf(
			"permission mask mismatch: consts %d, registry %d (a const was added without registering, or the registry holds a stale bit)",
			expectedMask,
			actualMask,
		)
	}
}

// TestPermissionRegistry_NoDuplicateBits asserts no bit is registered twice, which would let a later entry shadow an
// earlier one and mislabel permissions.
func TestPermissionRegistry_NoDuplicateBits(t *testing.T) {
	seen := make(map[int64]bool, len(permissionRegistry))
	for _, entry := range permissionRegistry {
		if seen[entry.Bit] {
			t.Errorf("permission bit %d registered more than once", entry.Bit)
		}

		seen[entry.Bit] = true
	}
}

func TestPermissionRegistry_NoZeroBit(t *testing.T) {
	for _, entry := range permissionRegistry {
		if entry.Bit == 0 {
			t.Errorf("permission entry %q has bit 0 (must be a power of two)", entry.Description)
		}
	}
}

// TestPermissionEntries_ReturnsAllRegisteredEntries asserts PermissionEntries returns one entry per registry row, in
// the same order, with matching bit and description.
func TestPermissionEntries_ReturnsAllRegisteredEntries(t *testing.T) {
	entries := PermissionEntries()

	if len(entries) != len(permissionRegistry) {
		t.Fatalf("entries: got %d, want %d (must match registry length)", len(entries), len(permissionRegistry))
	}

	for index, entry := range entries {
		if entry.Bit != permissionRegistry[index].Bit {
			t.Errorf("entries[%d].Bit: got %d, want %d", index, entry.Bit, permissionRegistry[index].Bit)
		}

		if entry.Description != permissionRegistry[index].Description {
			t.Errorf(
				"entries[%d].Description: got %q, want %q",
				index,
				entry.Description,
				permissionRegistry[index].Description,
			)
		}
	}
}

// TestPermissionEntries_ReturnsCopy asserts the returned slice is a copy: mutating it must not affect the package-level
// registry. A shared underlying array would let a caller corrupt the registry for every other consumer.
func TestPermissionEntries_ReturnsCopy(t *testing.T) {
	entries := PermissionEntries()

	if len(entries) == 0 {
		t.Fatal("entries: got empty, want non-empty (registry has entries)")
	}

	entries[0].Bit = -1
	entries[0].Description = "tampered"

	if permissionRegistry[0].Bit == -1 {
		t.Error("mutating returned slice modified the registry bit (must be a copy)")
	}

	if permissionRegistry[0].Description == "tampered" {
		t.Error("mutating returned slice modified the registry description (must be a copy)")
	}
}
