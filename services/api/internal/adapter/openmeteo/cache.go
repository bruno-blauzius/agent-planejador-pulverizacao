package openmeteo

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Clock is the time source of the cache.
type Clock interface {
	Now() time.Time
}

// Cache is a usecase.WeatherProvider decorator that keeps answers in memory
// per cell and local day for a TTL (addendum item 8): the fields of a farm
// share a cell, so one screen does not repeat the same request per field.
// Failures are not cached. Cached reports are shared and must not be modified.
type Cache struct {
	inner usecase.WeatherProvider
	ttl   time.Duration
	clock Clock
	loc   *time.Location

	mu      sync.Mutex
	entries map[cacheKey]cacheEntry
}

type cacheKey struct {
	cell string
	day  string
}

type cacheEntry struct {
	report  usecase.WeatherReport
	expires time.Time
}

// NewCache wraps inner; loc tells the local day that is part of the key.
func NewCache(inner usecase.WeatherProvider, ttl time.Duration, clock Clock, loc *time.Location) *Cache {
	return &Cache{inner: inner, ttl: ttl, clock: clock, loc: loc, entries: map[cacheKey]cacheEntry{}}
}

// Weather implements usecase.WeatherProvider.
func (c *Cache) Weather(ctx context.Context, cell domain.WeatherCell) (usecase.WeatherReport, error) {
	now := c.clock.Now()
	key := cacheKey{
		cell: strconv.FormatFloat(cell.Latitude, 'f', 2, 64) + "," + strconv.FormatFloat(cell.Longitude, 'f', 2, 64),
		day:  now.In(c.loc).Format(time.DateOnly),
	}
	c.mu.Lock()
	entry, ok := c.entries[key]
	c.mu.Unlock()
	if ok && now.Before(entry.expires) {
		return entry.report, nil
	}
	report, err := c.inner.Weather(ctx, cell)
	if err != nil {
		return usecase.WeatherReport{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpired(now)
	c.entries[key] = cacheEntry{report: report, expires: now.Add(c.ttl)}
	return report, nil
}

// evictExpired keeps the map bounded by the cells asked within one TTL.
func (c *Cache) evictExpired(now time.Time) {
	for key, entry := range c.entries {
		if !now.Before(entry.expires) {
			delete(c.entries, key)
		}
	}
}

var _ usecase.WeatherProvider = (*Cache)(nil)
