// Package executes Launchr application.
package main

import (
	"github.com/launchrctl/launchr"

	_ "github.com/launchrctl/scaffold"
)

func main() {
	launchr.RunAndExit()
}
