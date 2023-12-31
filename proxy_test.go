package proxy_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/razcoen/proxy"
	"github.com/stretchr/testify/assert"
)

func Test(t *testing.T) {
	callNumber := 0
	caller := proxy.Caller[proxy.StaticKeyable[int]]{
		CallFunc: func(ctx context.Context, v proxy.StaticKeyable[int]) (*proxy.Record, error) {
			fmt.Println("calling google")
			callNumber++
			if callNumber > 5 {
				return nil, fmt.Errorf("rate limit exceeded")
			}

			resp, err := http.Get("https://google.com")
			if err != nil {
				return nil, fmt.Errorf("http get: %w", err)
			}

			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("read body: %w", err)
			}

			return &proxy.Record{
				Data:           b,
				CacheStaleTime: time.Second,
				// BackupStaleTime: 7 * time.Second,
			}, nil
		},
	}

	for i := 0; i < 100; i++ {

		startTime := time.Now()
		b, err := caller.Call(context.Background(), proxy.NewKeyableStaticKey("google.com", i))
		fmt.Printf("CALL %d: duration: %s\n", i, time.Since(startTime))

		assert.NoError(t, err)
		assert.NotEmpty(t, b)
		//
		// if i%2 == 0 {
		// 	err = caller.InvalidateKey(context.Background(), "google.com")
		// 	assert.NoError(t, err)
		// }

		time.Sleep(100 * time.Millisecond)
	}
}

func Test2(t *testing.T) {
	record := proxy.PersistentRecord{
		Key:                 "1234",
		Data:                []byte("12345"),
		Timestamp:           time.Time{},
		CacheStaleDeadline:  time.Time{},
		BackupStaleDeadline: time.Time{},
		Invalidated:         false,
		CacheInvalidated:    false,
	}

	fmt.Println(record.SizeInBytes())
}
