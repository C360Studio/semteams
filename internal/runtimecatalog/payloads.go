package runtimecatalog

import (
	"errors"
	"fmt"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/governance"
	"github.com/c360studio/semstreams/graph/inference"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	gateddagexec "github.com/c360studio/semstreams/processor/gated-dag"
	"github.com/c360studio/semstreams/storage/objectstore"
)

// RegisterPayloads preserves the seven framework payload owners at SemStreams
// 8b99efe9c66a. Bootstrap layers product payloads on top. Internal lifecycle and
// hierarchy births need registrations even when no configured port names them.
// All owner errors are reported; callers must discard the registry on error.
func RegisterPayloads(registry *payloadregistry.Registry) error {
	if registry == nil {
		return errors.New("register payloads: registry cannot be nil")
	}
	owners := []struct {
		name     string
		register func(*payloadregistry.Registry) error
	}{
		{"message", message.RegisterPayloads},
		{"agentic", agentic.RegisterPayloads},
		{"processor/gated-dag", gateddagexec.RegisterPayloads},
		{"storage/objectstore", objectstore.RegisterPayloads},
		{"governance", governance.RegisterPayloads},
		{"pkg/lifecycle", lifecycle.RegisterPayloads},
		{"graph/inference", inference.RegisterPayloads},
	}
	var errs []error
	for _, owner := range owners {
		if err := owner.register(registry); err != nil {
			errs = append(errs, fmt.Errorf("register payload owner %q: %w", owner.name, err))
		}
	}
	return errors.Join(errs...)
}
