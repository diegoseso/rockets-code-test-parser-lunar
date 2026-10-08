package main

import (
	"os"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal"
	"github.com/diegoseso/rockets-code-test-parser-lunar/pkg/app"
	"github.com/diegoseso/rockets-code-test-parser-lunar/pkg/config"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func main() {
	// Defaults match the challenge statement: the test program posts to
	// http://localhost:8088/messages, so HTTP must listen on :8088 out of the box.
	cfg := config.Values{
		GrpcServer: ":8000",
		HTTP:       ":8088",
	}

	command, logger, ctx := app.New(internal.Name, &cfg)
	command.SilenceUsage = true
	command.RunE = func(*cobra.Command, []string) error {
		return internal.Bootstrap(ctx, cfg, logger)
	}

	if err := command.Execute(); err != nil {
		logger.Error(
			"unable to start backend service",
			zap.Error(err),
			zap.Stack("stacktrace"),
		)
		os.Exit(1)
	}
}
