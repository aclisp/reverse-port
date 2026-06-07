package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultListen            = ":9000"
	defaultStatusListen      = "127.0.0.1:9001"
	defaultOpenTimeout       = 10 * time.Second
	defaultReconnectInterval = 5 * time.Second
)

func main() {
	os.Exit(runMain(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func runMain(args []string, getenv func(string) string, _, stderr io.Writer) int {
	if len(args) == 0 {
		printTopUsage(stderr)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "server":
		cfg, ok := parseServerFlags(args[1:], getenv, stderr)
		if !ok {
			return 2
		}
		logger := log.New(stderr, "", log.LstdFlags)
		if err := RunServer(ctx, cfg, logger); err != nil {
			fmt.Fprintf(stderr, "server: %v\n", err)
			return 1
		}
		return 0
	case "client":
		cfg, ok := parseClientFlags(args[1:], getenv, stderr)
		if !ok {
			return 2
		}
		logger := log.New(stderr, "", log.LstdFlags)
		if err := RunClient(ctx, cfg, logger); err != nil {
			fmt.Fprintf(stderr, "client: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n", args[0])
		printTopUsage(stderr)
		return 2
	}
}

func parseServerFlags(args []string, getenv func(string) string, stderr io.Writer) (ServerConfig, bool) {
	cfg := ServerConfig{
		Listen:       defaultListen,
		StatusListen: defaultStatusListen,
		OpenTimeout:  defaultOpenTimeout,
	}
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var token string
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "tunnel listen address")
	fs.StringVar(&cfg.StatusListen, "status-listen", cfg.StatusListen, "loopback HTTP status listen address")
	fs.DurationVar(&cfg.OpenTimeout, "open-timeout", cfg.OpenTimeout, "pending data attach timeout")
	fs.StringVar(&token, "token", "", "shared authentication token")
	if err := fs.Parse(args); err != nil {
		serverUsage(stderr)
		return cfg, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "server does not accept positional arguments")
		serverUsage(stderr)
		return cfg, false
	}
	cfg.Token = tokenFromFlagOrEnv(token, getenv)
	if err := validateToken(cfg.Token); err != nil {
		fmt.Fprintf(stderr, "invalid token: %v\n", err)
		serverUsage(stderr)
		return cfg, false
	}
	if err := validateListenAddress(cfg.Listen, true); err != nil {
		fmt.Fprintf(stderr, "invalid --listen: %v\n", err)
		serverUsage(stderr)
		return cfg, false
	}
	if err := validateStatusListen(cfg.StatusListen); err != nil {
		fmt.Fprintf(stderr, "invalid --status-listen: %v\n", err)
		serverUsage(stderr)
		return cfg, false
	}
	if cfg.OpenTimeout <= 0 {
		fmt.Fprintln(stderr, "--open-timeout must be positive")
		serverUsage(stderr)
		return cfg, false
	}
	return cfg, true
}

func parseClientFlags(args []string, getenv func(string) string, stderr io.Writer) (ClientConfig, bool) {
	cfg := ClientConfig{ReconnectInterval: defaultReconnectInterval}
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var token, remote string
	fs.StringVar(&cfg.Server, "server", "", "server tunnel address")
	fs.StringVar(&remote, "remote", "", "server-side remote bind address")
	fs.StringVar(&cfg.Target, "target", "", "client-side target address")
	fs.StringVar(&token, "token", "", "shared authentication token")
	fs.DurationVar(&cfg.ReconnectInterval, "reconnect-interval", cfg.ReconnectInterval, "fixed reconnect interval")
	if err := fs.Parse(args); err != nil {
		clientUsage(stderr)
		return cfg, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "client does not accept positional arguments")
		clientUsage(stderr)
		return cfg, false
	}
	if cfg.Server == "" || remote == "" || cfg.Target == "" {
		fmt.Fprintln(stderr, "--server, --remote, and --target are required")
		clientUsage(stderr)
		return cfg, false
	}
	cfg.Token = tokenFromFlagOrEnv(token, getenv)
	if err := validateToken(cfg.Token); err != nil {
		fmt.Fprintf(stderr, "invalid token: %v\n", err)
		clientUsage(stderr)
		return cfg, false
	}
	normalizedRemote, err := normalizeRemoteAddress(remote)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --remote: %v\n", err)
		clientUsage(stderr)
		return cfg, false
	}
	cfg.Remote = normalizedRemote
	if err := validateDialAddress(cfg.Server); err != nil {
		fmt.Fprintf(stderr, "invalid --server: %v\n", err)
		clientUsage(stderr)
		return cfg, false
	}
	if err := validateDialAddress(cfg.Target); err != nil {
		fmt.Fprintf(stderr, "invalid --target: %v\n", err)
		clientUsage(stderr)
		return cfg, false
	}
	if cfg.ReconnectInterval <= 0 {
		fmt.Fprintln(stderr, "--reconnect-interval must be positive")
		clientUsage(stderr)
		return cfg, false
	}
	return cfg, true
}

func tokenFromFlagOrEnv(flagToken string, getenv func(string) string) string {
	if flagToken != "" {
		return flagToken
	}
	return getenv("RPORT_TOKEN")
}

func printTopUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: rpf <server|client> [flags]")
	serverUsage(w)
	clientUsage(w)
}

func serverUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: rpf server [--listen :9000] [--status-listen 127.0.0.1:9001] [--open-timeout 10s] [--token secret]")
}

func clientUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: rpf client --server host:port --remote [bind_address:]port --target host:hostport [--token secret] [--reconnect-interval 5s]")
}
