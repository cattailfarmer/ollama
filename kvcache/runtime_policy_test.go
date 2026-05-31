package kvcache

import "testing"

func TestRuntimePolicyDefaultOffNoOp(t *testing.T) {
	policy, err := NewRuntimePolicy(DefaultRuntimePolicyConfig())
	if err != nil {
		t.Fatal(err)
	}
	state := NewMemoryRuntimePolicyState()
	policy.State = state

	result, err := policy.Apply(RuntimePolicyRequest{
		References:      ReferenceSet{{Address: ReferenceAddress{Layer: ReferenceLayerSurface}}},
		CurrentPosition: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Mode, RuntimePolicyModeOff; got != want {
		t.Fatalf("mode = %q, want %q", got, want)
	}
	if len(result.Decisions) != 0 || len(result.Traces) != 0 || len(result.ContextPack) != 0 {
		t.Fatalf("disabled policy produced side effects: %+v", result)
	}
	if result.Mutated || result.RolledBack {
		t.Fatalf("disabled policy should be plain no-op: %+v", result)
	}
	if got := len(state.Applications()); got != 0 {
		t.Fatalf("disabled policy mutated runtime state %d times", got)
	}
}

func TestRuntimePolicyTraceOnlyRecordsWithoutMutation(t *testing.T) {
	ref := contractReference()
	state := NewMemoryRuntimePolicyState()
	policy := mustRuntimePolicy(t, RuntimePolicyModeTraceOnly, state, nil, nil)

	result, err := policy.Apply(RuntimePolicyRequest{
		References:      ReferenceSet{ref},
		CurrentPosition: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, want := len(result.Traces), 1; got != want {
		t.Fatalf("trace count = %d, want %d", got, want)
	}
	if result.Traces[0].CandidateAction != RetentionActionPinContract {
		t.Fatalf("unexpected candidate action: %+v", result.Traces[0])
	}
	if result.Mutated || len(state.Applications()) != 0 {
		t.Fatalf("trace-only policy mutated runtime state: %+v", result)
	}
}

func TestRuntimePolicySimulateOnlyMaterializesContextWithoutMutation(t *testing.T) {
	text := stringsJoinLines(
		"& [Policy] is source",
		"  = must: preserve raw fallback",
	)
	document := SourceDocument{Path: "policy.sop", Digest: SourceDigest(text), Text: text}
	annotations, err := SOPReferenceProducer{}.ProduceReferences(document)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewMemorySourceStore(document)
	if err != nil {
		t.Fatal(err)
	}
	rehydrator, err := NewRehydrator(store)
	if err != nil {
		t.Fatal(err)
	}
	state := NewMemoryRuntimePolicyState()
	policy := mustRuntimePolicy(t, RuntimePolicyModeSimulateOnly, state, nil, rehydrator)

	result, err := policy.Apply(RuntimePolicyRequest{
		References:      annotations.References(),
		CurrentPosition: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(result.ContextPack) == 0 {
		t.Fatal("expected simulate-only mode to materialize context pack evidence")
	}
	if result.Mutated || len(state.Applications()) != 0 {
		t.Fatalf("simulate-only policy mutated runtime state: %+v", result)
	}
	if !result.Traces[0].Rehydrated {
		t.Fatalf("expected first trace to record rehydration: %+v", result.Traces[0])
	}
}

func TestRuntimePolicyGuardedApplyMutatesAfterResolutionGate(t *testing.T) {
	ref := contractReference()
	resolver := NewMemoryReferenceStore()
	if err := resolver.PutReference(ref); err != nil {
		t.Fatal(err)
	}
	state := NewMemoryRuntimePolicyState()
	policy := mustRuntimePolicy(t, RuntimePolicyModeGuardedApply, state, resolver, nil)

	result, err := policy.Apply(RuntimePolicyRequest{
		References:      ReferenceSet{ref},
		CurrentPosition: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if !result.Mutated || len(state.Applications()) != 1 {
		t.Fatalf("guarded policy did not mutate through explicit state hook: %+v", result)
	}
	trace := result.Traces[0]
	if !trace.Resolved || !trace.Mutated || trace.Fallback != RuntimePolicyFallbackNone {
		t.Fatalf("unexpected guarded apply trace: %+v", trace)
	}
}

func TestRuntimePolicyFailedRehydrationFallsBackToPreserveRaw(t *testing.T) {
	text := "alpha\nbeta"
	document := SourceDocument{Path: "present.sop", Digest: SourceDigest(text), Text: text}
	ref := Reference{
		Address: ReferenceAddress{
			Layer: ReferenceLayerSurface,
			Source: &SourceAddress{
				Path:      document.Path,
				Digest:    document.Digest,
				BeginLine: 50,
				EndLine:   50,
			},
		},
	}
	resolver := NewMemoryReferenceStore()
	if err := resolver.PutReference(ref); err != nil {
		t.Fatal(err)
	}
	store, err := NewMemorySourceStore(document)
	if err != nil {
		t.Fatal(err)
	}
	rehydrator, err := NewRehydrator(store)
	if err != nil {
		t.Fatal(err)
	}
	state := NewMemoryRuntimePolicyState()
	policy := mustRuntimePolicy(t, RuntimePolicyModeGuardedApply, state, resolver, rehydrator)

	result, err := policy.Apply(RuntimePolicyRequest{
		References:      ReferenceSet{ref},
		CurrentPosition: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	trace := result.Traces[0]
	if trace.Fallback != RuntimePolicyFallbackPreserveRaw {
		t.Fatalf("fallback = %q, want %q", trace.Fallback, RuntimePolicyFallbackPreserveRaw)
	}
	if trace.FinalAction != RetentionActionRetainRawKV {
		t.Fatalf("final action = %q, want %q", trace.FinalAction, RetentionActionRetainRawKV)
	}
	if result.Mutated || len(state.Applications()) != 0 {
		t.Fatalf("failed rehydration must not mutate runtime state: %+v", result)
	}
}

func TestRuntimePolicyFailedResolutionFallsBackToPreserveRaw(t *testing.T) {
	ref := contractReference()
	state := NewMemoryRuntimePolicyState()
	policy := mustRuntimePolicy(t, RuntimePolicyModeGuardedApply, state, NewMemoryReferenceStore(), nil)

	result, err := policy.Apply(RuntimePolicyRequest{
		References:      ReferenceSet{ref},
		CurrentPosition: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	trace := result.Traces[0]
	if trace.Fallback != RuntimePolicyFallbackPreserveRaw {
		t.Fatalf("fallback = %q, want %q", trace.Fallback, RuntimePolicyFallbackPreserveRaw)
	}
	if trace.FinalAction != RetentionActionRetainRawKV {
		t.Fatalf("final action = %q, want %q", trace.FinalAction, RetentionActionRetainRawKV)
	}
	if result.Mutated || len(state.Applications()) != 0 {
		t.Fatalf("failed resolution must not mutate runtime state: %+v", result)
	}
}

func TestRuntimePolicyGuardedApplyRequiresResolver(t *testing.T) {
	ref := contractReference()
	state := NewMemoryRuntimePolicyState()
	policy := mustRuntimePolicy(t, RuntimePolicyModeGuardedApply, state, nil, nil)

	result, err := policy.Apply(RuntimePolicyRequest{
		References:      ReferenceSet{ref},
		CurrentPosition: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	trace := result.Traces[0]
	if trace.Fallback != RuntimePolicyFallbackPreserveRaw {
		t.Fatalf("fallback = %q, want %q", trace.Fallback, RuntimePolicyFallbackPreserveRaw)
	}
	if result.Mutated || len(state.Applications()) != 0 {
		t.Fatalf("missing resolver must not mutate runtime state: %+v", result)
	}
}

func TestRuntimePolicyRollbackToggleOffClearsNextRequest(t *testing.T) {
	ref := contractReference()
	resolver := NewMemoryReferenceStore()
	if err := resolver.PutReference(ref); err != nil {
		t.Fatal(err)
	}
	state := NewMemoryRuntimePolicyState()
	policy := mustRuntimePolicy(t, RuntimePolicyModeGuardedApply, state, resolver, nil)

	if _, err := policy.Apply(RuntimePolicyRequest{References: ReferenceSet{ref}, CurrentPosition: 64}); err != nil {
		t.Fatal(err)
	}
	if got := len(state.Applications()); got != 1 {
		t.Fatalf("initial guarded apply count = %d, want 1", got)
	}

	policy.Config.Mode = RuntimePolicyModeOff
	result, err := policy.Apply(RuntimePolicyRequest{References: ReferenceSet{ref}, CurrentPosition: 64})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Traces) != 0 || result.Mutated || result.RolledBack {
		t.Fatalf("disabled request after rollback toggle produced side effects: %+v", result)
	}
	if got := len(state.Applications()); got != 1 {
		t.Fatalf("disabled request changed runtime state count to %d", got)
	}
}

func mustRuntimePolicy(t *testing.T, mode RuntimePolicyMode, state RuntimePolicyState, resolver ReferenceResolver, rehydrator *Rehydrator) *RuntimePolicy {
	t.Helper()

	policy, err := NewRuntimePolicy(RuntimePolicyConfig{
		Mode:            mode,
		RetentionPolicy: DefaultRetentionPolicy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	policy.State = state
	policy.Resolver = resolver
	policy.Rehydrator = rehydrator

	return policy
}

func contractReference() Reference {
	return Reference{
		Address:    ReferenceAddress{Layer: ReferenceLayerContract, Anchor: "contract:runtime_policy"},
		Role:       "contract",
		Confidence: 1,
	}
}
