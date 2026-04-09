package natsclient

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

// NewKVStoreForTest creates a KVStore from a bare jetstream.KeyValue bucket
// without requiring a Client. This is intended for unit tests using mock
// KV implementations.
func NewKVStoreForTest(bucket jetstream.KeyValue, logger *slog.Logger) *KVStore {
	return &KVStore{
		bucket:  bucket,
		options: DefaultKVOptions(),
		logger:  logger,
	}
}
