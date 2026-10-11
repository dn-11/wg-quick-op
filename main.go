package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/dn-11/wg-quick-op/cmd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd.Execute(ctx)
}
