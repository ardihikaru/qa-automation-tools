package objectstore

import (
	"context"
	"sync"
	"time"
)

// fakeAPI is an in-memory api that records every call.
type fakeAPI struct {
	mu    sync.Mutex
	objs  map[string][]byte
	mod   map[string]time.Time
	calls []string
	err   error // returned by every call when set
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{objs: map[string][]byte{}, mod: map[string]time.Time{}}
}

func (f *fakeAPI) rec(op, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, op+" "+key)
	return f.err
}

func (f *fakeAPI) stat(_ context.Context, _, key string) (Info, error) {
	if err := f.rec("stat", key); err != nil {
		return Info{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objs[key]
	if !ok {
		return Info{}, noSuchKey()
	}
	return Info{Size: int64(len(b)), LastModified: f.mod[key]}, nil
}

func (f *fakeAPI) get(_ context.Context, _, key string, max int64) ([]byte, error) {
	if err := f.rec("get", key); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objs[key]
	if !ok {
		return nil, noSuchKey()
	}
	if int64(len(b)) > max {
		return nil, errTooLarge
	}
	return b, nil
}

func (f *fakeAPI) put(_ context.Context, _, key, _ string, data []byte) error {
	if err := f.rec("put", key); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objs[key] = append([]byte(nil), data...)
	f.mod[key] = time.Now()
	return nil
}
