package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
)

import "github.com/fruitbars/go-xfyun-cli/internal/cli"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "xfyun: %v\n", err)
		os.Exit(1)
	}
}
