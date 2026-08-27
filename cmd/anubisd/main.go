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

// version is stamped by the build: -ldflags="-X main.version=<sha>". The
// Dockerfile has passed it since day one — this variable is what it was
// always supposed to land in.
var version = "dev"

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
	case "baseline":
		err = runBaseline(ctx, logger)
	case "keys":
		err = runKeys(ctx, logger, os.Args[2:])
	case "bootstrap":
		err = runBootstrap(ctx, logger, os.Args[2:])
	// --version and -v are what people type on a host they have just been
	// handed; refusing them and then omitting `version` from the usage line
	// (as this did) sends them to the release page to guess.
	case "version", "--version", "-v":
		fmt.Println("anubisd " + version)
	default:
		err = fmt.Errorf("unknown command %q (serve|migrate|baseline|keys|bootstrap|version)", cmd)
	}
	if err != nil {
		logger.Error("fatal", "cmd", cmd, "error", err)
		os.Exit(1)
	}
}
