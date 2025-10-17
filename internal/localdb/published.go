package localdb

import (
	"time"
)

// PublishedService represents a saved published listener published via tsnet.
type PublishedService struct {
	ClusterID string    `json:"cluster_id"`
	Service   string    `json:"service"`
	Addr      string    `json:"addr"`
	AddedAt   time.Time `json:"added_at"`
}

const publishedCollection = "published_services"

// SavePublished saves or updates a published service record.
func (d *DB) SavePublished(key string, ps PublishedService) error {
	return d.Put(publishedCollection, key, ps)
}

// DeletePublished removes a published service record.
func (d *DB) DeletePublished(key string) error {
	return d.Delete(publishedCollection, key)
}

// ListPublished lists all published services.
func (d *DB) ListPublished(out *[]PublishedService) error {
	return d.List(publishedCollection, out)
}
