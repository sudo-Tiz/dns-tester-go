// Asynq worker entrypoint - delegates to cli.NewWorkerCommand.
package main

import (
	"fmt"
	"os"

	"github.com/sudo-tiz/dns-tester-go/internal/cli"
)

func main() {
	cmd := cli.NewWorkerCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
