package proxy

import (
	"context"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	defaultDataSource atomic.Pointer[DataSource]
)

func SetDefaultDataSoure(dataSource DataSource) {
	defaultDataSource.Store(&dataSource)
}

func DefaultDataSource() DataSource {
	return *defaultDataSource.Load()
}

func init() {
	SetDefaultDataSoure(NewLRUDataSource(SizeMB))
}

type Record struct {
	Key             string
	Data            []byte
	CacheStaleTime  time.Duration
	BackupStaleTime time.Duration
}

type PersistentRecord struct {
	Key                 string
	Data                []byte
	Timestamp           time.Time
	CacheStaleDeadline  time.Time
	BackupStaleDeadline time.Time
	Invalidated         bool
	CacheInvalidated    bool
}

func (record PersistentRecord) IsCacheStale() bool {
	return time.Since(record.CacheStaleDeadline) > 0
}

func (record PersistentRecord) IsBackupStale() bool {
	return time.Since(record.BackupStaleDeadline) > 0
}

func (record PersistentRecord) SizeInBytes() int {
	return int(unsafe.Sizeof(record)) +
		len([]byte(record.Key)) +
		len(record.Data)
}

type DataSource interface {
	Set(ctx context.Context, record PersistentRecord) error
	InvalidateKey(ctx context.Context, key string) error
	InvalidateCacheKey(ctx context.Context, key string) error
	Get(ctx context.Context, key string) (*PersistentRecord, error)
}

type ErrorStrategy int

const (
	ErrorStrategyReturnBackupAndError ErrorStrategy = iota
	ErrorStrategyReturnNilError
	ErrorStrategyReturnBackupAndNoError
)

type Keyable interface {
	Key() string
}

type CallFunc[T Keyable] func(ctx context.Context, keyables []T) ([]*Record, error)

type BackoffOptions struct {
	BackoffTimeout         time.Duration
	BackoffMultiplier      int
	BackoffInitialDuration time.Duration
}

type Caller[T Keyable] struct {
	CallFunc        CallFunc[T]
	DataSource      DataSource
	CacheStaleTime  time.Duration
	BackupStaleTime time.Duration
	BackoffOptions  BackoffOptions
	ErrorStrategy   ErrorStrategy
}

func (s *Caller[T]) dataSource() DataSource {
	if s.DataSource != nil {
		return s.DataSource
	}

	return DefaultDataSource()
}

func (s *Caller[T]) Call(ctx context.Context, keyables []T) (map[string][]byte, error) {
	recordByKey := make(map[string]*PersistentRecord)
	dataByKey := make(map[string][]byte)
	for _, keyable := range keyables {

		persistentRecord, err := s.dataSource().Get(ctx, keyable.Key())
		if err != nil {
			// TODO: Add logger
			persistentRecord = &PersistentRecord{Invalidated: true}
		}

		recordByKey[persistentRecord.Key] = persistentRecord
		isCacheRecordValid := err == nil && !persistentRecord.IsCacheStale() && !persistentRecord.CacheInvalidated && !persistentRecord.Invalidated
		if isCacheRecordValid {
			dataByKey[persistentRecord.Key] = persistentRecord.Data
		}
	}

	backoffTimeout := 10 * time.Second
	if s.BackoffOptions.BackoffTimeout != 0 {
		backoffTimeout = s.BackoffOptions.BackoffTimeout
	}

	backoffInitialDuraiton := 100 * time.Millisecond
	if s.BackoffOptions.BackoffInitialDuration != 0 {
		backoffInitialDuraiton = s.BackoffOptions.BackoffInitialDuration
	}

	backoffMultiplier := 2
	if s.BackoffOptions.BackoffMultiplier > 0 {
		backoffMultiplier = s.BackoffOptions.BackoffMultiplier
	}

	defaultCacheStaleTime := time.Minute
	if s.CacheStaleTime != 0 {
		defaultCacheStaleTime = s.CacheStaleTime
	}

	defaultBackupStaleTime := 30 * 24 * time.Hour
	if s.BackupStaleTime != 0 {
		defaultBackupStaleTime = s.BackupStaleTime
	}

	backoffContext, cancel := context.WithTimeout(ctx, backoffTimeout)
	defer cancel()

	sleepDuration := backoffInitialDuraiton
	var callError error
	var records []*Record
	var done bool
	for !done {
		records, callError = s.CallFunc(ctx, keyables)
		cacheStaleTime := defaultCacheStaleTime
		for _, record := range records {
			if record.CacheStaleTime != 0 {
				cacheStaleTime = record.CacheStaleTime
			}

			backupStaleTime := defaultBackupStaleTime
			if record.BackupStaleTime != 0 {
				backupStaleTime = record.BackupStaleTime
			}

			if callError == nil {
				timestamp := time.Now()
				err := s.dataSource().Set(ctx, PersistentRecord{
					Key:                 record.Key,
					Data:                record.Data,
					Timestamp:           timestamp,
					CacheStaleDeadline:  timestamp.Add(cacheStaleTime),
					BackupStaleDeadline: timestamp.Add(backupStaleTime),
					Invalidated:         false,
					CacheInvalidated:    false,
				})
				if err != nil {
					// TODO: Add logger
				}

				dataByKey[record.Key] = record.Data
			}
		}

		ticker := time.NewTicker(sleepDuration)
		select {
		case <-backoffContext.Done():
			done = true

		case <-ticker.C:
			sleepDuration *= time.Duration(backoffMultiplier)
		}
	}

	for _, keyable := range keyables {
		_, ok := dataByKey[keyable.Key()]
		if !ok {
			record := recordByKey[keyable.Key()]
			isValidBackupData := !record.IsBackupStale() && !record.Invalidated
			if isValidBackupData {
				dataByKey[keyable.Key()] = record.Data
			}
		}
	}

	return dataByKey, nil
}

func (s *Caller[T]) InvalidateKey(ctx context.Context, key string) error {
	return s.dataSource().InvalidateKey(ctx, key)
}

func (s *Caller[T]) InvalidateCacheKey(ctx context.Context, key string) error {
	return s.dataSource().InvalidateCacheKey(ctx, key)
}

func SimpleCall[T Keyable](ctx context.Context, keyables []T, callFunc CallFunc[T]) (map[string][]byte, error) {
	stub := Caller[T]{CallFunc: callFunc}
	return stub.Call(ctx, keyables)
}
