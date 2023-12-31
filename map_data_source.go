package proxy

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/exp/maps"
)

const (
	SizeKB uint64 = 1 << 10
	SizeMB uint64 = 1 << 20
	SizeGB uint64 = 1 << 30
)

var (
	_ DataSource = &LRUDataSource{}
)

type node struct {
	key      string
	previous *node
	next     *node
}

type accessQueue struct {
	head      *node
	tail      *node
	nodeByKey map[string]*node
}

func newAccessQueue() *accessQueue {
	return &accessQueue{
		nodeByKey: make(map[string]*node),
	}
}

func (aq *accessQueue) accessKey(key string) {
	if aq.head != nil && aq.head.key == key {
		return
	}

	aq.removeKey(key)
	aq.pushKey(key)
}

func (aq *accessQueue) removeKey(key string) {
	node, ok := aq.nodeByKey[key]
	if !ok {
		return
	}

	previous := node.previous
	next := node.next

	if previous != nil {
		previous.next = next
		if previous.next == nil {
			aq.tail = previous
		}
	}

	if next != nil {
		next.previous = previous
		if next.previous == nil {
			aq.head = next
		}
	}

	delete(aq.nodeByKey, key)
}

func (aq *accessQueue) pushKey(key string) {
	next := aq.head
	head := &node{previous: nil, next: next, key: key}
	if next != nil {
		next.previous = head
	}

	aq.nodeByKey[key] = head
	aq.head = head
	if aq.tail == nil {
		aq.tail = head
	}
}

func (aq *accessQueue) lastKey() (string, bool) {
	if aq.tail == nil {
		return "", false
	}

	return aq.tail.key, true
}

type LRUDataSource struct {
	mu              sync.Mutex
	kv              map[string]*PersistentRecord
	capacityInBytes uint64
	sizeInBytes     uint64
	accessQueue     *accessQueue
}

func NewLRUDataSource(capacityInBytes uint64) *LRUDataSource {
	return &LRUDataSource{
		mu:              sync.Mutex{},
		kv:              make(map[string]*PersistentRecord),
		capacityInBytes: capacityInBytes,
		sizeInBytes:     0,
		accessQueue:     newAccessQueue(),
	}
}

func (dsm *LRUDataSource) Get(ctx context.Context, key string) (*PersistentRecord, error) {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	v, ok := dsm.kv[key]
	if !ok {
		return nil, fmt.Errorf(`missing value for key %q`, key)
	}

	dsm.accessQueue.accessKey(key)
	return v, nil
}

func (dsm *LRUDataSource) InvalidateCacheKey(ctx context.Context, key string) error {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	v, ok := dsm.kv[key]
	if !ok {
		return fmt.Errorf(`missing value for key %q`, key)
	}

	v.CacheInvalidated = true
	dsm.kv[key] = v
	return nil
}

func (dsm *LRUDataSource) InvalidateKey(ctx context.Context, key string) error {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	v, ok := dsm.kv[key]
	if !ok {
		return fmt.Errorf(`missing value for key %q`, key)
	}

	v.Invalidated = true
	dsm.kv[key] = v
	return nil
}

func (dsm *LRUDataSource) Set(ctx context.Context, record PersistentRecord) error {
	fmt.Println(maps.Keys(dsm.kv))
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	previousRecord, ok := dsm.kv[record.Key]

	newSizeInBytes := dsm.sizeInBytes
	if ok {
		newSizeInBytes -= uint64(previousRecord.SizeInBytes())
	}

	dsm.accessQueue.accessKey(record.Key)
	newSizeInBytes += uint64(record.SizeInBytes())
	if newSizeInBytes > dsm.capacityInBytes {
		removedKey, _ := dsm.accessQueue.lastKey()
		dsm.accessQueue.removeKey(removedKey)
		removedRecord := dsm.kv[removedKey]
		if removedRecord == nil {
			panic("wtf")
		}
		delete(dsm.kv, removedKey)
		newSizeInBytes = dsm.sizeInBytes - uint64(removedRecord.SizeInBytes()) + uint64(record.SizeInBytes())
	}

	dsm.kv[record.Key] = &record
	dsm.sizeInBytes = newSizeInBytes
	return nil
}
