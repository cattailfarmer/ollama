package kvcache

import "testing"

func TestRetentionSimulatorDecidesDeterministically(t *testing.T) {
	addressSpace := AddressSpace{Model: ModelFingerprint{Architecture: "llama"}, Name: "llama.cpp", Version: "1"}
	refs := ReferenceSet{
		{Address: ReferenceAddress{Layer: ReferenceLayerContract, Anchor: "contract:Specification_Governance"}},
		{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Token: &TokenSpan{Sequence: 0, Begin: 80, End: 90}}},
		{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Token: &TokenSpan{Sequence: 0, Begin: 1, End: 2}, Source: &SourceAddress{Path: "AGENTS.md", Digest: "sha256:abc"}}},
		{Address: ReferenceAddress{Layer: ReferenceLayerSubtext, Anchor: "subtext:tone:uncertain"}},
		{Address: ReferenceAddress{Layer: ReferenceLayerReflective, AddressSpace: &addressSpace, Parameter: &ParameterAddress{Layer: 2, Module: "attention.k_proj", Tensor: "weight", Offset: 0, Size: 64}}},
	}
	simulator, err := NewRetentionSimulator(DefaultRetentionPolicy())
	if err != nil {
		t.Fatal(err)
	}

	decisions, err := simulator.Simulate(RetentionRequest{References: refs, CurrentPosition: 200})
	if err != nil {
		t.Fatal(err)
	}
	if err := decisions.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(decisions) != len(refs) {
		t.Fatalf("expected %d decisions, got %d", len(refs), len(decisions))
	}

	expected := []struct {
		lifecycle ReferenceLifecycle
		action    RetentionAction
	}{
		{ReferenceLifecyclePinned, RetentionActionPinContract},
		{ReferenceLifecycleMaterialized, RetentionActionRetainRawKV},
		{ReferenceLifecycleRehydratable, RetentionActionRetainSource},
		{ReferenceLifecycleCompressed, RetentionActionCompressToReference},
		{ReferenceLifecycleCompressed, RetentionActionAnnotate},
	}

	for i, want := range expected {
		if got := decisions[i].Lifecycle; got != want.lifecycle {
			t.Fatalf("decision %d lifecycle = %q, want %q", i, got, want.lifecycle)
		}
		if got := decisions[i].Action; got != want.action {
			t.Fatalf("decision %d action = %q, want %q", i, got, want.action)
		}
	}
}

func TestRetentionSimulatorCanPinReflectiveReferences(t *testing.T) {
	addressSpace := AddressSpace{Model: ModelFingerprint{Architecture: "llama"}, Name: "llama.cpp", Version: "1"}
	ref := Reference{
		Address: ReferenceAddress{
			Layer:        ReferenceLayerReflective,
			AddressSpace: &addressSpace,
			Activation:   &ActivationAddress{Layer: 2, Head: 1, Begin: 8, End: 16},
		},
	}
	simulator, err := NewRetentionSimulator(BasicRetentionPolicy{
		RecentTokenWindow: 32,
		PinReflective:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	decisions, err := simulator.Simulate(RetentionRequest{References: ReferenceSet{ref}, CurrentPosition: 64})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := decisions[0].Lifecycle, ReferenceLifecyclePinned; got != want {
		t.Fatalf("lifecycle = %q, want %q", got, want)
	}
	if got, want := decisions[0].Action, RetentionActionAnnotate; got != want {
		t.Fatalf("action = %q, want %q", got, want)
	}
}

func TestRetentionSimulatorRejectsInvalidRequests(t *testing.T) {
	simulator, err := NewRetentionSimulator(DefaultRetentionPolicy())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := simulator.Simulate(RetentionRequest{CurrentPosition: -1}); err == nil {
		t.Fatal("expected current position validation error")
	}

	_, err = simulator.Simulate(RetentionRequest{
		References: ReferenceSet{{Address: ReferenceAddress{Layer: ReferenceLayerSurface}}},
	})
	if err == nil {
		t.Fatal("expected reference validation error")
	}

	if _, err := NewRetentionSimulator(BasicRetentionPolicy{RecentTokenWindow: -1}); err == nil {
		t.Fatal("expected policy validation error")
	}
}

func TestRetentionDecisionStableKeyPartIncludesDecisionShape(t *testing.T) {
	ref := Reference{Address: ReferenceAddress{Layer: ReferenceLayerContract, Anchor: "contract:Specification_Governance"}}
	decision := retentionDecision(ref, ref.StableKey(), ReferenceLifecyclePinned, RetentionActionPinContract, "contract reference is pinned")

	if err := decision.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, want := decision.StableKeyPart(), "decision:reference=layer%3Dcontract%7Canchor%3Dcontract%253ASpecification_Governance:lifecycle=pinned:action=pin_contract:reason_len=28"; got != want {
		t.Fatalf("unexpected stable key part:\ngot:  %s\nwant: %s", got, want)
	}
}
