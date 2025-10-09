package permission

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func newCapability(name string, actions []string, matchLabels map[string]string) *unstructured.Unstructured {
	acts := make([]any, 0, len(actions))
	for _, a := range actions {
		acts = append(acts, a)
	}
	var sel map[string]any
	if len(matchLabels) > 0 {
		ml := map[string]any{}
		for k, v := range matchLabels {
			ml[k] = v
		}
		sel = map[string]any{"matchLabels": ml}
	}
	spec := map[string]any{"actions": acts}
	if sel != nil {
		spec["selector"] = sel
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "guildnet.io/v1alpha1",
		"kind":       "Capability",
		"metadata":   map[string]any{"name": name},
		"spec":       spec,
	}}
}

// TestAllowBasic verifies allow semantics with and without capabilities present.
func TestAllowBasic(t *testing.T) {
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "capabilities"}
	cl := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		gvr: "CapabilityList",
	})
	cache := NewCache(cl, "default", 50*time.Millisecond)
	if !cache.Allow(context.Background(), ActionStopAll, map[string]string{"team": "alpha"}) {
		// permissive when empty
		t.Fatalf("expected allow when no capabilities exist")
	}
	// Add a capability permitting only team=alpha for stopAll
	cap := newCapability("alpha-stop", []string{ActionStopAll}, map[string]string{"team": "alpha"})
	_, _ = cl.Resource(gvr).Namespace("default").Create(context.Background(), cap, metav1.CreateOptions{})
	// Force refresh
	cache.lastSync = time.Time{}
	if !cache.Allow(context.Background(), ActionStopAll, map[string]string{"team": "alpha"}) {
		t.Fatalf("expected allow for matching selector")
	}
	if cache.Allow(context.Background(), ActionStopAll, map[string]string{"team": "beta"}) {
		t.Fatalf("expected deny for non-matching selector")
	}
}
