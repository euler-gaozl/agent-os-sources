package main

import (
	"context"
	"github.com/euler-gaozl/agent-os-sdk/servicehost"
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"github.com/euler-gaozl/agent-os-sources/sources"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
)

func main() {
	servicehost.Main(servicehost.Options{Role: "sources", Port: "8101", Database: true, Concurrency: 4, Authorize: sources.Authorize, Readiness: sources.Readiness}, func(ctx context.Context, p *pgxpool.Pool, c servicehttp.Config) (http.Handler, func(context.Context) error, error) {
		if err := applyLocalDefaults(); err != nil {
			return nil, nil, err
		}
		hosts := strings.Split(servicehost.Env("SOURCE_ALLOWED_HOSTS", strings.Join(sources.DefaultHosts, ",")), ",")
		return (&sources.Store{DB: p, AllowedHosts: hosts}).Handler(c), nil, nil
	})
}
