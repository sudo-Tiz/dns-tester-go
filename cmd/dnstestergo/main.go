// Main CLI entrypoint with query, server, worker subcommands.
package main

import (
	"github.com/sudo-tiz/dns-tester-go/internal/cli"
)

func main() {
	cli.Execute()
}
