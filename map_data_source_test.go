package proxy_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/razcoen/proxy"
)

func TestLRUDataSource(t *testing.T) {
	ds := proxy.NewLRUDataSource(proxy.SizeKB)
	for i := 0; i < 10000; i++ {
		fmt.Println(i)
		ds.Get(context.Background(), strconv.FormatInt(int64(i%3), 10))
		ds.Set(context.Background(), proxy.PersistentRecord{Key: strconv.FormatInt(int64(i), 10)})
	}
}
