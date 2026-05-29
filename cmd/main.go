package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Flgado/trino-insights-mcp/internal/timcp"
	"github.com/Flgado/trino-insights-mcp/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const envPrefix = "TRINO_INSIGHTS"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := newRoot()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	v := viper.New()
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	cmd := &cobra.Command{
		Use:   "trino-insights-mcp",
		Short: "MCP server that turns LLM agents into a trino performance copilot",
		Long: "Open-source Go MCP server. Speaks the Model Context Protocol over stdio, " +
			"fetches data from Trino's REST/UI APIs, projects it into compact agent-friendly DTOs, " +
			"and exposes tools for per-query deep dive. Read-only by default; never submits or cancels SQL.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	registerSharedFlags(cmd, v)
	cmd.AddCommand(newStdioCmd(v))

	return cmd
}

func registerSharedFlags(cmd *cobra.Command, v *viper.Viper) {
	pf := cmd.PersistentFlags()

	pf.String("coordinator-url", "", "Trino coordinator base URL (https://host[:port]). REQUIRED.")
	pf.String("user", "", "Trino user [X-Trino-User header]. Default: insights.")
	pf.String("password", "", "HTTP basic password.")
	pf.String("token", "", "Bearer token (mutually exclusive with --password).")
	pf.Bool("insecure-skip-tls-verify", false, "Skip TLS verification. Don't use it in prod.")
	pf.Duration("timeout", 15*time.Second, "HTTP request timeout per call to Trino.")

	pf.StringSlice("toolsets", nil, "Comma-separated toolset IDs to enable (default: use per-toolset Default flag)")
	pf.StringSlice("tools", nil, "Additional tool names to enable beyond toolsets.")
	pf.StringSlice("exclude-tools", nil, "Tool names to disable")
	pf.Bool("read-only", true, "Disable write tools (default: true)")
	pf.Int("content-window-size", 16*1024, "Per-tool soft cap on response payload bytes.")

	pf.Duration("queryinfo-cache-ttl", 5*time.Minute, "TTL for cached QueryInfo responses on terminal queries. 0 disables caching.")
	pf.Int("queryinfo-cache-size", 256, "Max QueryInfo entries kept in memory")

	pf.String("log-file", "", "Log to a file instead of stderr.")

	for _, key := range []string{
		"coordinator-url", "user", "password", "token", "insecure-skip-tls-verify", "timeout",
		"toolsets", "tools", "exclude-tools", "read-only", "content-window-size",
		"queryinfo-cache-ttl", "queryinfo-cache-size", "log-file",
	} {
		if err := v.BindPFlag(key, pf.Lookup(key)); err != nil {
			fmt.Fprintf(os.Stderr, "error binding flag %q: %v\n", key, err)
			os.Exit(1)
		}
	}
}

func loadConfig(v *viper.Viper) *config.Config {
	return &config.Config{
		Coordinator: config.Coordinator{
			BaseURL:               v.GetString("coordinator-url"),
			User:                  v.GetString("user"),
			Password:              v.GetString("password"),
			Token:                 v.GetString("token"),
			InsecureSkipTLSVerify: v.GetBool("insecure-skip-tls-verify"),
			Timeout:               v.GetDuration("timeout"),
		},
		Toolsets:           v.GetStringSlice("toolsets"),
		Tools:              v.GetStringSlice("tools"),
		ExcludeTools:       v.GetStringSlice("exclude-tools"),
		ReadOnly:           v.GetBool("read-only"),
		ContentWindowSize:  v.GetInt("content-window-size"),
		QueryInfoCacheTTL:  v.GetDuration("queryinfo-cache-ttl"),
		QueryInfoCacheSize: v.GetInt("queryinfo-cache-size"),
		LogFile:            v.GetString("log-file"),
		Version:            timcp.Version,
	}
}

func newStdioCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "stdio",
		Short: "Run the MCP server over stdin/stdout",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return timcp.RunStdio(cmd.Context(), loadConfig(v))
		},
	}
}
