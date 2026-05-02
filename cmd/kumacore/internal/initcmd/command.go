// Package initcmd implements standalone app generation for `kumacore init`.
package initcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const initUsage = "Usage: kumacore init [project-name]"

type usageErrorMessage struct {
	message string
}

func (usageError usageErrorMessage) Error() string {
	return usageError.message + "\n" + initUsage
}

func (usageError usageErrorMessage) Is(target error) bool {
	return target == errUsage
}

// Run parses init arguments and initializes the current repository.
func Run(args []string) {
	if err := run(args, os.Stdin, os.Stdout); err != nil {
		exitCode := 1
		if errors.Is(err, errUsage) {
			exitCode = 2
		}

		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(exitCode)
	}
}

var errUsage = errors.New("usage")

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 1 {
		return usageError("kumacore init: too many positional arguments")
	}

	projectNameArgument := ""
	if len(args) == 1 {
		projectNameArgument = args[0]
	}

	isInteractive, err := stdinIsInteractive(stdin)
	if err != nil {
		return fmt.Errorf("[initcmd:run] inspect stdin: %w", err)
	}

	if !isInteractive && strings.TrimSpace(projectNameArgument) == "" {
		return usageError("kumacore init: missing <project-name>")
	}

	reader := bufio.NewReader(stdin)
	resolvedOptions, err := resolveOptions(
		reader,
		stdout,
		isInteractive,
		projectNameArgument,
	)
	if err != nil {
		if errors.Is(err, errUsage) {
			return err
		}

		return fmt.Errorf("kumacore init: %w", err)
	}

	if err := validateOptions(resolvedOptions); err != nil {
		return fmt.Errorf("kumacore init: %w", err)
	}

	if err := ensureRepoRoot(); err != nil {
		return fmt.Errorf("kumacore init: %w", err)
	}

	if err := initializeRepository(stdout, resolvedOptions); err != nil {
		return fmt.Errorf("kumacore init: %w", err)
	}

	return nil
}

func usageError(message string) error {
	return usageErrorMessage{message: message}
}

func resolveOptions(
	reader *bufio.Reader,
	stdout io.Writer,
	isInteractive bool,
	projectNameArgument string,
) (Options, error) {
	projectName := strings.TrimSpace(projectNameArgument)
	if projectName == "" {
		if !isInteractive {
			return Options{}, usageError("kumacore init: missing <project-name>")
		}

		resolvedValue, err := promptString(reader, stdout, "project name", defaultProjectName())
		if err != nil {
			return Options{}, err
		}
		projectName = resolvedValue
	}

	moduleIDs := defaultModules()
	if isInteractive {
		if _, err := fmt.Fprintf(
			stdout,
			"available modules: %s (default: %s)\n",
			strings.Join(moduleOrder, ", "),
			strings.Join(defaultModules(), ","),
		); err != nil {
			return Options{}, fmt.Errorf("[initcmd:resolveOptions] write module list: %w", err)
		}

		resolvedValue, err := promptString(reader, stdout, "modules", strings.Join(defaultModules(), ","))
		if err != nil {
			return Options{}, err
		}

		moduleIDs = parseModuleList(resolvedValue)
	}

	return Options{
		ProjectName: projectName,
		Modules:     moduleIDs,
	}, nil
}

func parseModuleList(raw string) []string {
	parts := strings.Split(raw, ",")
	moduleIDs := make([]string, 0, len(parts))
	for _, part := range parts {
		moduleID := strings.TrimSpace(part)
		if moduleID == "" {
			continue
		}

		moduleIDs = append(moduleIDs, moduleID)
	}

	if len(moduleIDs) == 0 {
		return defaultModules()
	}

	return moduleIDs
}

func promptString(reader *bufio.Reader, stdout io.Writer, label string, defaultValue string) (string, error) {
	if _, err := fmt.Fprintf(stdout, "%s [%s]: ", label, defaultValue); err != nil {
		return "", fmt.Errorf("[initcmd:promptString] write prompt: %w", err)
	}

	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("[initcmd:promptString] read input: %w", err)
	}

	resolvedValue := strings.TrimSpace(line)
	if resolvedValue == "" {
		return defaultValue, nil
	}

	return resolvedValue, nil
}

func stdinIsInteractive(stdin io.Reader) (bool, error) {
	stdinFile, ok := stdin.(*os.File)
	if !ok {
		return false, nil
	}

	stdinInfo, err := stdinFile.Stat()
	if err != nil {
		return false, err
	}

	return stdinInfo.Mode()&os.ModeCharDevice != 0, nil
}
