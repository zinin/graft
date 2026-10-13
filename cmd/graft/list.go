package main

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/zinin/graft/internal/config"
)

// list prints the profiles in file order.
func list(cfg *config.Config, sys system) int {
	tw := tabwriter.NewWriter(sys.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROFILE\tMODE\tMODEL\tSUBSCRIPTION\tPROVIDERS")
	for _, name := range cfg.ProfileOrder {
		p := cfg.Profiles[name]
		sub, providers := "no", strings.Join(cfg.ProvidersOf(p), ", ")
		if p.Subscription {
			sub = "yes"
		}
		if providers == "" {
			providers = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", name, cfg.ModeOf(p), p.Model, sub, providers)
	}
	tw.Flush()
	return 0
}
