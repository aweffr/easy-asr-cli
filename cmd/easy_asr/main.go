package main

import (
	"os"

	"github.com/aweffr/easy-asr-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
