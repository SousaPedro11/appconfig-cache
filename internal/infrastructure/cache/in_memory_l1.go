package cache

import (
	"sync"
	"time"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

type memoryEntry struct {
	value     domain.Configuration
	expiresAt time.Time
}

type InMemoryL1Cache struct {
	entries sync.Map
	ttl     time.Duration
}

func NewInMemoryL1Cache(ttl time.Duration) *InMemoryL1Cache {
	return &InMemoryL1Cache{ttl: ttl}
}

func (c *InMemoryL1Cache) Get(key domain.CacheKey) (domain.Configuration, bool) {
	value, ok := c.entries.Load(key.String())
	if !ok {
		return domain.Configuration{}, false
	}

	entry := value.(memoryEntry)

	if time.Now().After(entry.expiresAt) {
		c.entries.Delete(key.String())
		return domain.Configuration{}, false
	}

	return entry.value, true
}

func (c *InMemoryL1Cache) Set(key domain.CacheKey, value domain.Configuration) {
	c.entries.Store(key.String(), memoryEntry{value: value, expiresAt: time.Now().Add(c.ttl)})
}
