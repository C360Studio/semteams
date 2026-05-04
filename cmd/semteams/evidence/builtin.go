package evidence

// RegisterBuiltins populates r with every kind this package ships
// out-of-the-box (R3.7.2.i′). Operators or product wiring code that
// build their own Registry call this once at boot to avoid the
// per-kind Register boilerplate; deployments wanting to filter the
// kind set construct an empty Registry and Register selectively.
//
// Kinds shipped:
//   - test_file_exists (universal — workspace path presence)
//   - test_uses_build_tag (Go — build constraint check)
//   - surefire_passing_count (Maven — surefire pass-count gate)
//
// Returns the first registration error so a typo'd duplicate Kind
// (extremely unlikely from this trusted-list path) surfaces at
// boot rather than at gate run.
func RegisterBuiltins(r *Registry) error {
	for _, ent := range []struct {
		kind    string
		checker Checker
	}{
		{KindTestFileExists, FileExistsChecker{}},
		{KindTestUsesBuildTag, BuildTagChecker{}},
		{KindSurefirePassingCount, SurefirePassingCountChecker{}},
	} {
		if err := r.Register(ent.kind, ent.checker); err != nil {
			return err
		}
	}
	return nil
}

// NewWithBuiltins is a convenience: empty Registry + RegisterBuiltins.
// Tests + production wiring share this; operators wanting a curated
// subset bypass it.
func NewWithBuiltins() (*Registry, error) {
	r := NewRegistry()
	if err := RegisterBuiltins(r); err != nil {
		return nil, err
	}
	return r, nil
}
