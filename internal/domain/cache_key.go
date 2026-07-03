package domain

import "fmt"

type CacheKey struct {
	value string
}

func NewCacheKey(application ApplicationID, environment EnvironmentID) CacheKey {
	return CacheKey{value: fmt.Sprintf("appconfig:%s:%s", application, environment)}
}

func (k CacheKey) String() string {
	return k.value
}
