package metrics

import (
	"sync/atomic"
	"time"
)

// Simple in-memory instrumentation (placeholder until Prometheus integration).
// Provides atomic counters per org/table/operation and gauge for active changefeed subscribers.

type key struct{ org, table, op string }
type keyC struct{ cluster, org, table, op string }

var (
	opCounts          syncMap[key, uint64]
	activeChangefeeds atomic.Int64
	opCountsC         syncMap[keyC, uint64]
)

// syncMap is a tiny generic wrapper using atomic.Value for copy-on-write maps.
type syncMap[K comparable, V any] struct{ m atomic.Value } // stores map[K]V

func (s *syncMap[K, V]) load() map[K]V {
	if v := s.m.Load(); v != nil {
		return v.(map[K]V)
	}
	return map[K]V{}
}
func (s *syncMap[K, V]) swap(m map[K]V) { s.m.Store(m) }

// IncOp increments an operation counter.
func IncOp(org, table, op string, delta uint64) {
	if delta == 0 {
		delta = 1
	}
	for {
		cur := opCounts.load()
		next := make(map[key]uint64, len(cur)+1)
		for k, v := range cur {
			next[k] = v
		}
		k := key{org: org, table: table, op: op}
		next[k] = next[k] + delta
		opCounts.swap(next)
		return
	}
}

// IncOpCluster increments an operation counter labeled with cluster_id.
func IncOpCluster(clusterID, org, table, op string, delta uint64) {
	if delta == 0 {
		delta = 1
	}
	for {
		cur := opCountsC.load()
		next := make(map[keyC]uint64, len(cur)+1)
		for k, v := range cur {
			next[k] = v
		}
		k := keyC{cluster: clusterID, org: org, table: table, op: op}
		next[k] = next[k] + delta
		opCountsC.swap(next)
		return
	}
}

// ChangefeedInc increments active changefeed gauge.
func ChangefeedInc() { activeChangefeeds.Add(1) }

// ChangefeedDec decrements active changefeed gauge.
func ChangefeedDec() { activeChangefeeds.Add(-1) }

// Snapshot returns all metrics as a simple structure.
type Snapshot struct {
	Timestamp   time.Time         `json:"ts"`
	Ops         map[string]uint64 `json:"ops"`
	Changefeeds int64             `json:"changefeeds"`
}

func Export() Snapshot {
	cur := opCounts.load()
	flat := make(map[string]uint64, len(cur))
	for k, v := range cur {
		flat[k.org+"/"+k.table+"/"+k.op] = v
	}
	// Merge per-cluster ops with a prefix
	curC := opCountsC.load()
	for k, v := range curC {
		flat["cluster/"+k.cluster+"/"+k.org+"/"+k.table+"/"+k.op] = v
	}
	return Snapshot{Timestamp: time.Now(), Ops: flat, Changefeeds: activeChangefeeds.Load()}
}
