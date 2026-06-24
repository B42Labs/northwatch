// Command ovnsim is a load generator and topology simulator for the Northwatch
// local OVN lab (see lab/). It writes directly to OVN Northbound over OVSDB.
//
//	ovnsim seed   — create an idempotent baseline topology (the "Grundlast")
//	ovnsim run    — continuously mutate that topology (the "Bewegung")
//	ovnsim clean  — delete everything ovnsim created
//
// Every object ovnsim creates is tagged so `run` and `clean` only ever touch
// the simulator's own rows.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/b42labs/northwatch/internal/ovnsim"
	"github.com/ovn-kubernetes/libovsdb/client"
)

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "seed":
		err = cmdSeed(args)
	case "run":
		err = cmdRun(args)
	case "bind":
		err = cmdBind(args, false)
	case "unbind":
		err = cmdBind(args, true)
	case "clean":
		err = cmdClean(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "ovnsim: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		// Each command wraps its own context into err; cmd is omitted from the
		// format string to avoid a tainted-value log-injection warning.
		log.Fatalf("ovnsim: %v", err)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `ovnsim — OVN Northbound load generator for the Northwatch lab

Usage:
  ovnsim seed   [flags]   create an idempotent baseline topology
  ovnsim run    [flags]   continuously create/delete switches, routers, ports, NAT, ACLs, LB VIPs
  ovnsim bind   [flags]   bind every seeded VIF onto a chassis (clears "unbound VIF" alerts)
  ovnsim unbind [flags]   remove the chassis binding from every seeded VIF
  ovnsim clean  [flags]   delete every object ovnsim created

Run "ovnsim <command> -h" for command-specific flags.
`)
}

// withNB parses common flags, connects to NB and runs fn against the client.
func withNB(fs *flag.FlagSet, args []string, nb *string, fn func(context.Context, client.Client) error) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c, closeFn, err := ovnsim.ConnectNB(connectCtx, *nb)
	if err != nil {
		return err
	}
	defer closeFn()
	return fn(ctx, c)
}

func cmdSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	nb := fs.String("nb", envOr("NORTHWATCH_OVN_NB_ADDR", "tcp:127.0.0.1:6641"), "OVN Northbound OVSDB address")
	switches := fs.Int("switches", 6, "number of tenant logical switches")
	routers := fs.Int("routers", 3, "number of logical routers")
	portsPer := fs.Int("ports-per-switch", 5, "VIF ports per switch")
	scale := fs.Float64("scale", 1.0, "multiply switches/routers/ports by this factor")
	chassis := fs.String("chassis", "chassis-1,chassis-2,chassis-3", "comma-separated chassis names for gateway/HA chassis")

	return withNB(fs, args, nb, func(ctx context.Context, c client.Client) error {
		opts := ovnsim.Options{
			Switches:       scaled(*switches, *scale),
			Routers:        scaled(*routers, *scale),
			PortsPerSwitch: scaled(*portsPer, *scale),
			Chassis:        splitCSV(*chassis),
		}
		log.Printf("seeding: %d switches, %d routers, %d ports/switch", opts.Switches, opts.Routers, opts.PortsPerSwitch)
		res, err := ovnsim.Seed(ctx, c, opts)
		if err != nil {
			return err
		}
		log.Printf("seed complete: %d rows created", res.Total())
		for _, t := range sortedKeys(res.Created) {
			log.Printf("  %-28s %d", t, res.Created[t])
		}
		return nil
	})
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	nb := fs.String("nb", envOr("NORTHWATCH_OVN_NB_ADDR", "tcp:127.0.0.1:6641"), "OVN Northbound OVSDB address")
	interval := fs.Duration("interval", 3*time.Second, "delay between simulator actions")
	target := fs.Int("target", 6, "target number of switches (homeostasis anchor)")
	randSeed := fs.Int64("rand-seed", 1, "PRNG seed for reproducible action sequences")
	once := fs.Bool("once", false, "perform a single action and exit")
	bindPorts := fs.Bool("bind-ports", false, "also bind/migrate ports onto chassis via docker exec (needs the lab running)")
	labName := fs.String("lab-name", "nw-lab", "containerlab lab name (used to address chassis containers)")
	chassis := fs.String("chassis", "chassis-1,chassis-2,chassis-3", "comma-separated chassis names")

	return withNB(fs, args, nb, func(ctx context.Context, c client.Client) error {
		cfg := ovnsim.SimConfig{
			Options:  ovnsim.Options{Switches: *target, Routers: 3, PortsPerSwitch: 5, Chassis: splitCSV(*chassis)},
			Target:   *target,
			RandSeed: *randSeed,
			Logf:     func(f string, a ...any) { log.Printf(f, a...) },
		}
		if *bindPorts {
			cfg.Binder = ovnsim.NewBinder(*labName, splitCSV(*chassis))
			log.Printf("port binding enabled via docker exec on lab %q", *labName)
			// Bind any already-unbound VIFs up front (e.g. freshly seeded ones)
			// so the lab starts healthy; the loop then keeps new ports bound.
			if n, err := ovnsim.BindAll(ctx, c, cfg.Binder); err != nil {
				log.Printf("initial bind: %v", err)
			} else if n > 0 {
				log.Printf("bound %d pre-existing unbound VIFs", n)
			}
		}
		sim := ovnsim.NewSimulator(c, cfg)

		if *once {
			desc, err := sim.Step(ctx)
			if err != nil {
				return err
			}
			log.Printf("%s", desc)
			return nil
		}

		log.Printf("simulating every %s (target %d switches); Ctrl-C to stop", *interval, *target)
		err := sim.Run(ctx, *interval)
		if err != nil && err != context.Canceled {
			return err
		}
		return nil
	})
}

func cmdBind(args []string, unbind bool) error {
	name := "bind"
	if unbind {
		name = "unbind"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	nb := fs.String("nb", envOr("NORTHWATCH_OVN_NB_ADDR", "tcp:127.0.0.1:6641"), "OVN Northbound OVSDB address")
	labName := fs.String("lab-name", "nw-lab", "containerlab/compose lab name (used to address chassis containers)")
	chassis := fs.String("chassis", "chassis-1,chassis-2,chassis-3", "comma-separated chassis names")

	return withNB(fs, args, nb, func(ctx context.Context, c client.Client) error {
		binder := ovnsim.NewBinder(*labName, splitCSV(*chassis))
		if unbind {
			n, err := ovnsim.UnbindAll(ctx, c, binder)
			log.Printf("unbound %d VIFs", n)
			return err
		}
		n, err := ovnsim.BindAll(ctx, c, binder)
		log.Printf("bound %d VIFs across chassis %v", n, splitCSV(*chassis))
		return err
	})
}

func cmdClean(args []string) error {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	nb := fs.String("nb", envOr("NORTHWATCH_OVN_NB_ADDR", "tcp:127.0.0.1:6641"), "OVN Northbound OVSDB address")
	return withNB(fs, args, nb, func(ctx context.Context, c client.Client) error {
		n, err := ovnsim.Clean(ctx, c)
		log.Printf("removed %d simulator-owned root objects", n)
		return err
	})
}

// scaled multiplies n by factor, rounding to the nearest integer, with a floor
// of 1 when the inputs are positive.
func scaled(n int, factor float64) int {
	if factor <= 0 {
		return n
	}
	v := int(math.Round(float64(n) * factor))
	if v < 1 && n > 0 {
		v = 1
	}
	return v
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
