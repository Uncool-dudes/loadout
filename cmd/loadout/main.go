package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Uncool-dudes/loadout/internal/cli"
)

func main() {
	os.Exit(run())
}

// run exists so deferred cleanup runs before os.Exit; calling os.Exit in main
// would skip the signal-context stop.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.NewRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "loadout:", err)
		return 1
	}
	return 0
}
