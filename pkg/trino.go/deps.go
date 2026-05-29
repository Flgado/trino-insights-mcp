package trino

import (
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
	"github.com/Flgado/trino-insights-mcp/pkg/rest"
	"github.com/Flgado/trino-insights-mcp/pkg/translations"
)

type ToolDependencies struct {
	REST              *rest.Client
	Fetcher           queryinfo.Fetcher
	T                 translations.HelperFunc
	ContentWindowSize int
	ReadOnly          bool
}

func (d ToolDependencies) QueryFetcher() queryinfo.Fetcher {
	if d.Fetcher != nil {
		return d.Fetcher
	}
	return &queryinfo.RestFetcher{Client: d.REST}
}
