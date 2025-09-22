package svcinit

import (
	"context"
	"fmt"
	"sync"
)

func InitDataFromContext(ctx context.Context, name string) (any, bool) {
	id, ok := initDataFromContext(ctx)
	if !ok {
		return nil, false
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	dt, ok := id.data[name]
	if !ok {
		return nil, false
	}
	if !dt.isSet {
		return nil, false
	}
	return dt.value, true
}

func InitDataTypeFromContext[T any](ctx context.Context, name string) (T, bool) {
	id, ok := InitDataFromContext(ctx, name)
	if ok {
		if v, ok := id.(T); ok {
			return v, true
		}
	}
	var empty T
	return empty, false
}

func InitDataSet[T any](ctx context.Context, name string, data T) error {
	id, ok := initDataFromContext(ctx)
	if !ok {
		return fmt.Errorf("InitDataSet: no data found in context")
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	dt, ok := id.data[name]
	if !ok {
		return fmt.Errorf("InitDataSet: unknown data %s", name)
	}
	if dt.isSet {
		return fmt.Errorf("InitDataSet: data %s already set", name)
	}
	dt.isSet = true
	dt.value = data
	return nil
}

func initDataFromContext(ctx context.Context) (*initData, bool) {
	if val := ctx.Value(initDataKey{}); val != nil {
		if id, ok := val.(*initData); ok {
			return id, true
		}
	}
	return nil, false
}

type initDataKey struct{}

type initData struct {
	mu   sync.Mutex
	data map[string]*initDataItem
}

type initDataItem struct {
	isSet bool
	value any
}

func contextWithInitData(ctx context.Context, names []string) context.Context {
	id := initData{
		data: make(map[string]*initDataItem, len(names)),
	}
	for _, name := range names {
		id.data[name] = &initDataItem{}
	}
	return context.WithValue(ctx, causeKey{}, &id)
}
