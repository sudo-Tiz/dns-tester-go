// DNS Tester API server entrypoint - delegates to cli.NewServerCommand.
//
//go:generate go run github.com/swaggo/swag/cmd/swag@latest init -g cmd/api/main.go -o internal/api/docs --parseDependency --parseInternal
package main

import (
	"fmt"
	"os"

	_ "github.com/sudo-tiz/dns-tester-go/internal/api/docs" // swagger docs
	"github.com/sudo-tiz/dns-tester-go/internal/cli"
)

// @title DNS-Tester-GO API
// @version 1.0.0
// @description Asynchronous DNS testing with support for Do53, DoT, DoH, DoQ protocols
// @description This API allows you to submit DNS queries and retrieve results asynchronously
//
// @contact.name DNS-Tester-GO
// @contact.url https://github.com/sudo-Tiz/DNS-Tester-GO
// @contact.email contact@example.com
//
// @license.name MIT
// @license.url https://github.com/sudo-Tiz/DNS-Tester-GO/blob/main/LICENSE
//
// @host localhost:5000
// @BasePath /
// @schemes http https
//
// @tag.name DNS
// @tag.description DNS lookup operations
// @tag.name Tasks
// @tag.description Task management and status retrieval
// @tag.name System
// @tag.description System health and metrics
func main() {
	cmd := cli.NewServerCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
