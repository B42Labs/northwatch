package enrich

import (
	"context"
	"time"
)

// Info holds enrichment information for an OVN entity.
type Info struct {
	DisplayName string            `json:"display_name,omitempty"`
	ProjectName string            `json:"project_name,omitempty"`
	ProjectID   string            `json:"project_id,omitempty"`
	DeviceOwner string            `json:"device_owner,omitempty"`
	DeviceID    string            `json:"device_id,omitempty"`
	DeviceName  string            `json:"device_name,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// Provider enriches OVN entities with external information.
type Provider interface {
	Name() string
	EnrichPort(ctx context.Context, externalIDs map[string]string) (*Info, error)
	EnrichNetwork(ctx context.Context, externalIDs map[string]string) (*Info, error)
	EnrichRouter(ctx context.Context, externalIDs map[string]string) (*Info, error)
	EnrichNAT(ctx context.Context, externalIDs map[string]string) (*Info, error)
}

// Enricher wraps a Provider with caching. A nil provider makes all methods no-op.
type Enricher struct {
	provider Provider
	cache    *Cache
}

// NewEnricher creates an Enricher. If provider is nil, the Enricher is a no-op.
func NewEnricher(provider Provider, ttl time.Duration) *Enricher {
	var cache *Cache
	if provider != nil {
		cache = NewCache(ttl)
	}
	return &Enricher{
		provider: provider,
		cache:    cache,
	}
}

// HasProvider returns true if an enrichment provider is configured.
func (e *Enricher) HasProvider() bool {
	return e.provider != nil
}

// EnrichPort enriches a port entity. The id is used as cache key.
func (e *Enricher) EnrichPort(ctx context.Context, id string, externalIDs map[string]string) *Info {
	return e.enrich(ctx, "port:"+id, func() (*Info, error) {
		return e.provider.EnrichPort(ctx, externalIDs)
	})
}

// EnrichNetwork enriches a network entity. The id is used as cache key.
func (e *Enricher) EnrichNetwork(ctx context.Context, id string, externalIDs map[string]string) *Info {
	return e.enrich(ctx, "network:"+id, func() (*Info, error) {
		return e.provider.EnrichNetwork(ctx, externalIDs)
	})
}

// EnrichRouter enriches a router entity. The id is used as cache key.
func (e *Enricher) EnrichRouter(ctx context.Context, id string, externalIDs map[string]string) *Info {
	return e.enrich(ctx, "router:"+id, func() (*Info, error) {
		return e.provider.EnrichRouter(ctx, externalIDs)
	})
}

// EnrichNAT enriches a NAT entity. The id is used as cache key.
func (e *Enricher) EnrichNAT(ctx context.Context, id string, externalIDs map[string]string) *Info {
	return e.enrich(ctx, "nat:"+id, func() (*Info, error) {
		return e.provider.EnrichNAT(ctx, externalIDs)
	})
}

func (e *Enricher) enrich(ctx context.Context, cacheKey string, fn func() (*Info, error)) *Info {
	if e.provider == nil {
		return nil
	}

	if info, ok := e.cache.Get(cacheKey); ok {
		return info
	}

	info, err := fn()
	if err != nil || info == nil {
		return nil
	}

	e.cache.Set(cacheKey, info)
	return info
}
