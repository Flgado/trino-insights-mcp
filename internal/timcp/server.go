package timcp

import (
	"context"
	"fmt"
	"os"

	"github.com/Flgado/trino-insights-mcp/pkg/config"
	"github.com/Flgado/trino-insights-mcp/pkg/inventory"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
	"github.com/Flgado/trino-insights-mcp/pkg/rest"
	"github.com/Flgado/trino-insights-mcp/pkg/translations"
	trino "github.com/Flgado/trino-insights-mcp/pkg/trino.go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var Version = "dev"

func RunStdio(ctx context.Context, cfg *config.Config) error {
	server, _, err := buildServer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to build server: %w", err)
	}

	errC := make(chan error, 1)
	go func() {
		errC <- server.Run(ctx, &mcp.IOTransport{
			Reader: os.Stdin,
			Writer: os.Stdout,
		})
	}()

	_, _ = fmt.Fprintf(os.Stderr, "Trino Insights MCP Server running on stdio\n")

	select {
	case <-ctx.Done():
		return nil
	case err := <-errC:
		return err
	}
}

func pickAuth(cfg *config.Config) rest.Authenticator {
	switch {
	case cfg.Coordinator.Token != "":
		return rest.BearerAuth{User: cfg.Coordinator.User, Token: cfg.Coordinator.Token}
	case cfg.Coordinator.Password != "":
		return rest.BasicAuth{User: cfg.Coordinator.User, Password: cfg.Coordinator.Password}
	default:
		return rest.NoAuth{User: cfg.Coordinator.User}
	}
}

func buildServer(ctx context.Context, cfg *config.Config) (*mcp.Server, *inventory.Inventory, error) {
	cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid config: %w", err)
	}

	auth := pickAuth(cfg)
	client, err := rest.NewClient(rest.Options{
		BaseURL:               cfg.Coordinator.BaseURL,
		Timeout:               cfg.Coordinator.Timeout,
		InsecureSkipTLSVerify: cfg.Coordinator.InsecureSkipTLSVerify,
		Auth:                  auth,
		UserAgent:             "trino-insights-mcp/" + cfg.Version,
	})
	if err != nil {
		return nil, nil, err
	}

	t, _ := translations.NewHelper()

	var fetcher queryinfo.Fetcher = &queryinfo.RestFetcher{Client: client}
	if cfg.QueryInfoCacheTTL > 0 {
		fetcher = queryinfo.NewCachedFetcher(fetcher, cfg.QueryInfoCacheTTL, cfg.QueryInfoCacheSize)
	}

	deps := trino.ToolDependencies{
		REST:              client,
		Fetcher:           fetcher,
		T:                 t,
		ContentWindowSize: cfg.ContentWindowSize,
		ReadOnly:          cfg.ReadOnly,
	}

	inv, err := inventory.NewBuilder().
		SetTools(trino.AllTools(t)).
		WithToolsets(cfg.Toolsets).
		WithTools(cfg.Tools).
		WithExclude(cfg.ExcludeTools).
		WithReadOnly(cfg.ReadOnly).
		Build()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build inventory: %w", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "trino-insights-mcp",
		Title:   "Trino Insights MCP Server",
		Version: cfg.Version,
	}, &mcp.ServerOptions{
		Instructions: inv.Instructions(),
	})

	inv.RegisterAll(ctx, server, deps)

	return server, inv, nil
}
