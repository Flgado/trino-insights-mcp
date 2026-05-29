package queryinfo

import (
	"context"
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/errors"
	"github.com/Flgado/trino-insights-mcp/pkg/rest"
)

type sentinelError string

func (e sentinelError) Error() string {
	return string(e)
}

const errMissingQueryID = sentinelError("query ID is required")

type Fetcher interface {
	Fetch(ctx context.Context, queryID string) (*QueryInfo, error)
}

type RestFetcher struct {
	Client *rest.Client
}

func (f *RestFetcher) Fetch(ctx context.Context, queryID string) (*QueryInfo, error) {
	if queryID == "" {
		return nil, errMissingQueryID
	}
	var qi QueryInfo
	if err := f.Client.GetJSON(ctx, fmt.Sprintf("/v1/query/%s", queryID), &qi); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("query %q not found — Trino purges QueryInfo after a short window; the query may have expired", queryID)
		}
		return nil, err
	}
	return &qi, nil
}
