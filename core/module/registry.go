package module

import (
	"fmt"
	"sort"
	"strings"
)

// Registry stores explicitly registered app-local modules.
type Registry struct {
	modulesByID map[string]Module
}

// NewRegistry creates a registry and registers the provided modules.
func NewRegistry(modules ...Module) (*Registry, error) {
	registry := &Registry{modulesByID: make(map[string]Module)}
	if err := registry.Register(modules...); err != nil {
		return nil, err
	}

	return registry, nil
}

// Register appends modules supplied by generated bootstrap code.
func (registry *Registry) Register(modules ...Module) error {
	if registry.modulesByID == nil {
		registry.modulesByID = make(map[string]Module)
	}

	for _, appModule := range modules {
		if appModule == nil {
			return fmt.Errorf("[module:Register] nil module")
		}

		moduleID := strings.TrimSpace(appModule.ID())
		if moduleID == "" {
			return fmt.Errorf("[module:Register] empty module ID")
		}

		if _, exists := registry.modulesByID[moduleID]; exists {
			return fmt.Errorf("[module:Register] duplicate module ID %q", moduleID)
		}

		registry.modulesByID[moduleID] = appModule
	}

	return nil
}

// Resolve returns modules in deterministic ID order.
func (registry *Registry) Resolve() []Module {
	moduleIDs := registry.sortedModuleIDs()
	resolvedModules := make([]Module, 0, len(moduleIDs))

	for _, moduleID := range moduleIDs {
		resolvedModules = append(resolvedModules, registry.modulesByID[moduleID])
	}

	return resolvedModules
}

// Contributions resolves modules and collects their registrations.
func (registry *Registry) Contributions() (*Contributions, error) {
	return CollectContributions(registry.Resolve()...)
}

// CollectContributions collects registrations from modules in caller-provided order.
func CollectContributions(modules ...Module) (*Contributions, error) {
	contributions := &Contributions{}
	for _, appModule := range modules {
		if err := appModule.Register(contributions); err != nil {
			return nil, fmt.Errorf("[module:CollectContributions] register %s: %w", appModule.ID(), err)
		}
	}

	return contributions, nil
}

func (registry *Registry) sortedModuleIDs() []string {
	moduleIDs := make([]string, 0, len(registry.modulesByID))
	for moduleID := range registry.modulesByID {
		moduleIDs = append(moduleIDs, moduleID)
	}
	sort.Strings(moduleIDs)

	return moduleIDs
}
