// kumacore initializes this repository as a Go and HTMX application.
package main

import (
	"fmt"
	"os"

	"kumacore/cmd/kumacore/internal/initcmd"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	command := os.Args[1]
	switch command {
	case "init":
		initcmd.Run(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "kumacore: unknown command %q\n", command)
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println("Usage: kumacore init [project-name]")
	fmt.Println()
	fmt.Println("Generate a standalone app with selected modules.")
	fmt.Println()
}
