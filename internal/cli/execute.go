package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aweffr/easy-asr-cli/internal/config"
	"github.com/aweffr/easy-asr-cli/internal/engine"
)

const (
	ExitOK         = 0
	ExitUnexpected = 1
	ExitUsage      = 2
	ExitInput      = 3
	ExitTimeout    = 7
)

func Execute(args []string, stdout io.Writer, stderr io.Writer) int {
	cmd := NewRootCommand(Deps{Stdout: stdout, Stderr: stderr})
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil {
		return ExitOK
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return ExitCode(err)
}

func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	message := err.Error()
	if config.IsUsageError(err) ||
		engine.IsUsageError(err) ||
		strings.Contains(message, "unknown command") ||
		strings.Contains(message, "unknown flag") ||
		strings.Contains(message, "requires") ||
		strings.Contains(message, "accepts") {
		return ExitUsage
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return ExitInput
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ExitTimeout
	}
	return ExitUnexpected
}
