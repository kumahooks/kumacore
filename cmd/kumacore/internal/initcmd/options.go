package initcmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Options holds the resolved init command configuration.
type Options struct {
	ProjectName string
	Modules     []string
}

type RepoMetadata struct {
	Modules []ModuleSelection
}

type ModuleSelection struct {
	ID string
}

func defaultProjectName() string {
	return "kumacore"
}

func defaultModules() []string {
	return []string{"home"}
}

func validateProjectName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("[initcmd] project name is required")
	}

	if trimmed == "." || trimmed == ".." {
		return fmt.Errorf("[initcmd] project name %q is invalid", name)
	}

	if filepath.Base(trimmed) != trimmed {
		return fmt.Errorf("[initcmd] project name %q is invalid", name)
	}

	if strings.ContainsAny(trimmed, `\ /`) {
		return fmt.Errorf("[initcmd] project name %q is invalid", name)
	}

	for index, character := range trimmed {
		if !isAllowedProjectNameCharacter(character) {
			return fmt.Errorf("[initcmd] project name %q is invalid", name)
		}

		if index == 0 && !unicode.IsLower(character) {
			return fmt.Errorf("[initcmd] project name %q is invalid", name)
		}
	}

	lastCharacter := rune(trimmed[len(trimmed)-1])
	if !(unicode.IsLower(lastCharacter) || unicode.IsDigit(lastCharacter)) {
		return fmt.Errorf("[initcmd] project name %q is invalid", name)
	}

	return nil
}

func isAllowedProjectNameCharacter(character rune) bool {
	return unicode.IsLower(character) || unicode.IsDigit(character) || character == '.' || character == '-' ||
		character == '_'
}

func validateOptions(options Options) error {
	if err := validateProjectName(options.ProjectName); err != nil {
		return err
	}

	if err := validateModules(options.Modules); err != nil {
		return err
	}

	return nil
}

func metadataFromOptions(options Options) RepoMetadata {
	metadata := RepoMetadata{
		Modules: make([]ModuleSelection, 0, len(options.Modules)),
	}

	for _, moduleID := range normalizeModuleIDs(options.Modules) {
		metadata.Modules = append(metadata.Modules, ModuleSelection{ID: moduleID})
	}

	return metadata
}
