package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/sousapedro11/appconfig-cache/internal/domain"
)

func TestInMemoryL1CacheGetSetAndExpiration(t *testing.T) {
	cache := NewInMemoryL1Cache(10 * time.Millisecond)
	key := domain.NewCacheKey(domain.ApplicationID("app"), domain.EnvironmentID("prd"))

	if _, ok := cache.Get(key); ok {
		t.Fatalf("cache deveria estar vazia")
	}

	config, _ := domain.NewConfiguration("value")
	cache.Set(key, config)
	if got, ok := cache.Get(key); !ok || got.Content() != "value" {
		t.Fatalf("valor inesperado: got=%q ok=%v", got.Content(), ok)
	}

	time.Sleep(15 * time.Millisecond)
	if _, ok := cache.Get(key); ok {
		t.Fatalf("cache deveria ter expirado")
	}
}

func TestInMemoryL1CacheConcurrentAccess(t *testing.T) {
	cache := NewInMemoryL1Cache(time.Second)
	key := domain.NewCacheKey(domain.ApplicationID("app"), domain.EnvironmentID("prd"))
	config, _ := domain.NewConfiguration("value")
	cache.Set(key, config)

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if got, ok := cache.Get(key); !ok || got.Content() != "value" {
					t.Errorf("read concurrente inconsistente: got=%q ok=%v", got.Content(), ok)
					return
				}
				cache.Set(key, config)
			}
		}()
	}

	wg.Wait()
}
