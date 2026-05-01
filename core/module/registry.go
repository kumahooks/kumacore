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
			return fmt.Errorf("[module] register: nil module")
		}

		moduleID := strings.TrimSpace(appModule.ID())
		if moduleID == "" {
			return fmt.Errorf("[module] register: empty module ID")
		}

		if _, exists := registry.modulesByID[moduleID]; exists {
			return fmt.Errorf("[module] register: duplicate module ID %q", moduleID)
		}

		registry.modulesByID[moduleID] = appModule
	}

	return nil
}

// Resolve returns modules in deterministic dependency order.
func (registry *Registry) Resolve() ([]Module, error) {
	moduleIDs := registry.sortedModuleIDs()
	visitStateByID := make(map[string]int, len(moduleIDs))
	resolvedModules := make([]Module, 0, len(moduleIDs))

	for _, moduleID := range moduleIDs {
		if err := registry.resolveModule(moduleID, visitStateByID, nil, &resolvedModules); err != nil {
			return nil, err
		}
	}

	return resolvedModules, nil
}

// Contributions resolves modules and collects their registrations.
func (registry *Registry) Contributions() (*Contributions, error) {
	resolvedModules, err := registry.Resolve()
	if err != nil {
		return nil, err
	}

	contributions := &Contributions{}
	for _, appModule := range resolvedModules {
		if err := appModule.Register(contributions); err != nil {
			return nil, fmt.Errorf("[module] register %s: %w", appModule.ID(), err)
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

func (registry *Registry) resolveModule(
	moduleID string,
	visitStateByID map[string]int,
	path []string,
	resolvedModules *[]Module,
) error {
	const visiting = 1
	const visited = 2

	if visitStateByID[moduleID] == visited {
		return nil
	}

	if visitStateByID[moduleID] == visiting {
		cyclePath := append(path, moduleID)
		return fmt.Errorf("[module] resolve: dependency cycle: %s", strings.Join(cyclePath, " -> "))
	}

	appModule, exists := registry.modulesByID[moduleID]
	if !exists {
		return fmt.Errorf("[module] resolve: missing module %q", moduleID)
	}

	visitStateByID[moduleID] = visiting
	dependencies := append([]string(nil), appModule.Manifest().DependsOn...)
	sort.Strings(dependencies)

	for _, dependencyID := range dependencies {
		dependencyID = strings.TrimSpace(dependencyID)
		if dependencyID == "" {
			return fmt.Errorf("[module] resolve: %s declares empty dependency", moduleID)
		}

		if _, exists := registry.modulesByID[dependencyID]; !exists {
			return fmt.Errorf("[module] resolve: %s depends on missing module %q", moduleID, dependencyID)
		}

		if err := registry.resolveModule(
			dependencyID,
			visitStateByID,
			append(path, moduleID),
			resolvedModules,
		); err != nil {
			return err
		}
	}

	visitStateByID[moduleID] = visited
	*resolvedModules = append(*resolvedModules, appModule)

	return nil
}
