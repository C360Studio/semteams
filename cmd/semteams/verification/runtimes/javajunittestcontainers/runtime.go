package javajunittestcontainers

import (
	"github.com/c360studio/semteams/cmd/semteams/verification/runtimes"
)

// Name is the registry key for this runtime. Hyphenated lowercase
// ASCII; matches the conventional shape architects emit on
// commitment.runtime.
const Name = "java-junit-testcontainers"

// invokeCommand is the bash shape the builder runs from inside the
// sandbox to execute tests. Contains a `<harness-profile>`
// placeholder the builder substitutes from the catalog entry's
// compose_profile field at invocation time.
const invokeCommand = "mvn verify -P<harness-profile>"

// Runtime implements runtimes.Runtime for Java + JUnit 5 +
// Testcontainers. R3.7.2.d ships metadata only; per-family
// templates wire in R3.7.2.f.
type Runtime struct {
	// templates maps family FullName → template body. Empty in v1;
	// R3.7.2.f populates "tcp.binary-protobuf.v1" with the JUnit
	// integration-test template.
	templates map[string]string
}

// New returns a new Runtime with no templates wired. R3.7.2.f
// extends this constructor (or adds a NewWithTemplates) to thread
// the family→template map at boot.
func New() *Runtime {
	return &Runtime{
		templates: map[string]string{},
	}
}

// Name implements runtimes.Runtime.
func (*Runtime) Name() string { return Name }

// Description implements runtimes.Runtime.
func (*Runtime) Description() string {
	return "Java with JUnit 5 + Testcontainers for sidecar lifecycle management. Builder runs `mvn verify -P<harness-profile>` from inside the sandbox; the rendered template uses Testcontainers' GenericContainer to wait-for-port the sidecar before exercising it. First runtime in the matrix."
}

// InvokeCommand implements runtimes.Runtime.
func (*Runtime) InvokeCommand() string { return invokeCommand }

// TemplateFor implements runtimes.Runtime. Returns ("", false) for
// all families in the R3.7.2.d posture; R3.7.2.f wires
// tcp.binary-protobuf.v1.
func (r *Runtime) TemplateFor(familyFullName string) (string, bool) {
	t, ok := r.templates[familyFullName]
	return t, ok
}

// SupportedFamilies implements runtimes.Runtime. Returns empty slice
// in the R3.7.2.d posture.
func (r *Runtime) SupportedFamilies() []string {
	if len(r.templates) == 0 {
		return nil
	}
	out := make([]string, 0, len(r.templates))
	for k := range r.templates {
		out = append(out, k)
	}
	return out
}

// Register registers this runtime on the supplied registry. Call
// from cmd/semteams/main.go after creating the registry.
func Register(reg *runtimes.Registry) error {
	return reg.Register(New())
}
