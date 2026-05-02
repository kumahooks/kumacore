package initcmd

import "fmt"

var moduleOrder = []string{"home", "auth", "health"}

// KnownModule returns true if id is a known scaffold module.
func KnownModule(id string) bool {
	switch id {
	case "home", "auth", "health":
		return true
	default:
		return false
	}
}

func validateModules(ids []string) error {
	seenModules := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !KnownModule(id) {
			return fmt.Errorf("[initcmd] unknown module %q", id)
		}

		if _, exists := seenModules[id]; exists {
			return fmt.Errorf("[initcmd] duplicate module %q", id)
		}

		seenModules[id] = struct{}{}
	}

	return nil
}

func normalizeModuleIDs(ids []string) []string {
	return normalizeSelection(ids, moduleOrder)
}

func normalizeSelection(ids []string, orderedIDs []string) []string {
	selectedIDs := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		selectedIDs[id] = struct{}{}
	}

	normalizedIDs := make([]string, 0, len(selectedIDs))
	for _, orderedID := range orderedIDs {
		if _, exists := selectedIDs[orderedID]; exists {
			normalizedIDs = append(normalizedIDs, orderedID)
		}
	}

	return normalizedIDs
}
