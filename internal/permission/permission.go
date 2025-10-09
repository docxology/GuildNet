package permission

import (
	"context"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Action constants (prototype).
const (
	ActionLaunch   = "launch"
	ActionDelete   = "delete"
	ActionStopAll  = "stopAll"
	ActionReadLogs = "readLogs"
	ActionProxy    = "proxy"
)

var capabilityGVR = schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "capabilities"}

// CapabilityEntry represents a simplified cached capability.
type CapabilityEntry struct {
	Name     string
	Actions  map[string]struct{}
	Selector labels.Selector
	// Constraints ignored for prototype
}

// Cache maintains a periodically refreshed capability list.
type Cache struct {
	dyn       dynamic.Interface
	namespace string
	mu        sync.RWMutex
	caps      []CapabilityEntry
	lastSync  time.Time
	syncEvery time.Duration
}

// NewCache creates a new capability cache. Namespace can be empty to list across namespaces (cluster-scope not used here).
func NewCache(dyn dynamic.Interface, namespace string, refresh time.Duration) *Cache {
	return &Cache{dyn: dyn, namespace: namespace, syncEvery: refresh}
}

// sync refreshes capabilities if stale.
func (c *Cache) sync(ctx context.Context) {
	c.mu.RLock()
	if time.Since(c.lastSync) < c.syncEvery {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()
	list, err := c.dyn.Resource(capabilityGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return // silent fail; permissive mode covers availability
	}
	var entries []CapabilityEntry
	for _, item := range list.Items {
		spec, ok := item.Object["spec"].(map[string]any)
		if !ok {
			continue
		}
		rawActions, _ := spec["actions"].([]any)
		acts := map[string]struct{}{}
		for _, a := range rawActions {
			if s, ok := a.(string); ok {
				acts[s] = struct{}{}
			}
		}
		var sel labels.Selector = labels.Everything()
		if selMap, ok := spec["selector"].(map[string]any); ok {
			// support only matchLabels subset for prototype
			if ml, ok := selMap["matchLabels"].(map[string]any); ok {
				lm := labels.Set{}
				for k, v := range ml {
					if vs, ok := v.(string); ok {
						lm[k] = vs
					}
				}
				sel = labels.SelectorFromSet(lm)
			}
		}
		entries = append(entries, CapabilityEntry{Name: item.GetName(), Actions: acts, Selector: sel})
	}
	c.mu.Lock()
	c.caps = entries
	c.lastSync = time.Now()
	c.mu.Unlock()
}

// Allow returns true if the action is permitted for a workspace with the given labels.
// Prototype behavior: if no capability objects exist, allow everything.
func (c *Cache) Allow(ctx context.Context, action string, wsLabels map[string]string) bool {
	c.sync(ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.caps) == 0 {
		return true
	}
	lbls := labels.Set(wsLabels)
	for _, cap := range c.caps {
		if _, ok := cap.Actions[action]; !ok {
			continue
		}
		if cap.Selector.Matches(lbls) {
			return true
		}
	}
	return false
}
