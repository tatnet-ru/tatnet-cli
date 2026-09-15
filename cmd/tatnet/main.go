// Команда tatnet — консольный клиент публичного API TatNet.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/tatnet-ru/tatnet-cli/internal/cli"
)

// Подставляются линковщиком при сборке релиза.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	root := cli.NewRootCommand(versionString())
	if err := root.Execute(); err != nil {
		if !errors.Is(err, cli.ErrAborted) {
			fmt.Fprintln(os.Stderr, "Ошибка: "+err.Error())
		}
		os.Exit(1)
	}
}

func versionString() string {
	s := version
	if commit != "" {
		s += " (" + commit
		if date != "" {
			s += ", " + date
		}
		s += ")"
	}
	return s
}
