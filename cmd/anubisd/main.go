// anubisd is the Anubis daemon: serve | migrate | keys | bootstrap.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = runServe(ctx, logger)
	case "migrate":
		err = runMigrate(ctx, logger)
	case "keys":
		err = runKeys(ctx, logger, os.Args[2:])
	case "bootstrap":
		err = runBootstrap(ctx, logger, os.Args[2:])
	case "version":
		fmt.Println("anubisd dev")
	default:
		err = fmt.Errorf("unknown command %q (serve|migrate|keys|bootstrap)", cmd)
	}
	if err != nil {
		logger.Error("fatal", "cmd", cmd, "error", err)
		os.Exit(1)
	}
}
