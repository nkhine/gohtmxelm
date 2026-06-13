// Package dynamo is a pure-Go, in-process table that emulates a single-key
// DynamoDB table: CreateTableIfNotExists, PutItem (upsert by partition key),
// Count, Scan, and Recent. It needs no Docker, DynamoDB Local, or AWS — the
// data lives in memory. A real DynamoDB-backed implementation could satisfy the
// same shape later without changing callers.
package dynamo

import (
	"fmt"
	"sort"
	"sync"
)

// Item is a DynamoDB-style attribute map.
type Item map[string]any

// Table is an in-memory single-key table. PutItem upserts by the partition-key
// attribute; Scan returns every item; Recent returns the newest items by the
// table's numeric sort attribute.
type Table struct {
	mu    sync.RWMutex
	name  string
	pk    string // partition-key attribute name
	sort  string // numeric attribute ordering Recent (newest first)
	items map[string]Item
}

// PutItem upserts item by its partition-key value (a non-empty string attribute
// named by the table's partition key).
func (t *Table) PutItem(item Item) error {
	key, ok := item[t.pk].(string)
	if !ok || key == "" {
		return fmt.Errorf("dynamo: item missing string partition key %q", t.pk)
	}
	t.mu.Lock()
	t.items[key] = item
	t.mu.Unlock()
	return nil
}

// Count returns the number of items in the table.
func (t *Table) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.items)
}

// Scan returns a snapshot of every item (unordered).
func (t *Table) Scan() []Item {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Item, 0, len(t.items))
	for _, it := range t.items {
		out = append(out, it)
	}
	return out
}

// Recent returns up to limit items, newest first by the sort attribute.
func (t *Table) Recent(limit int) []Item {
	out := t.Scan()
	sort.Slice(out, func(i, j int) bool {
		return AsInt64(out[i][t.sort]) > AsInt64(out[j][t.sort])
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// AsInt64 reads an integer attribute regardless of its concrete numeric type.
func AsInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

// DB is an in-memory DynamoDB-style account: a set of named tables.
type DB struct {
	mu     sync.Mutex
	tables map[string]*Table
}

// NewDB returns an empty in-memory account.
func NewDB() *DB {
	return &DB{tables: make(map[string]*Table)}
}

// CreateTableIfNotExists returns the named table, creating it with the given
// partition key and numeric sort attribute if absent (idempotent, mirroring a
// CreateTable guarded by DescribeTable).
func (db *DB) CreateTableIfNotExists(name, partitionKey, sortAttr string) *Table {
	db.mu.Lock()
	defer db.mu.Unlock()
	if t, ok := db.tables[name]; ok {
		return t
	}
	t := &Table{name: name, pk: partitionKey, sort: sortAttr, items: make(map[string]Item)}
	db.tables[name] = t
	return t
}
