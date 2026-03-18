package main

import (
	"flag"

	"github.com/snapetech/iptvtunerr/internal/config"
)

type commandSpec struct {
	Name    string
	Section string
	Summary string
	FlagSet *flag.FlagSet
	Run     func(cfg *config.Config, args []string)
}
