package kvcache

import "testing"

func TestRehydratorRecoversExactSourceSpan(t *testing.T) {
	text := "alpha\nbeta\ngamma"
	document := SourceDocument{Path: "source.sop", Digest: SourceDigest(text), Text: text}
	store, err := NewMemorySourceStore(document)
	if err != nil {
		t.Fatal(err)
	}
	rehydrator, err := NewRehydrator(store)
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{
		Address: ReferenceAddress{
			Layer:  ReferenceLayerSurface,
			Source: &SourceAddress{Path: document.Path, Digest: document.Digest, BeginLine: 2, EndLine: 2},
		},
	}
	decision := retentionDecision(ref, ref.StableKey(), ReferenceLifecycleRehydratable, RetentionActionRetainSource, "source preserved")

	item, err := rehydrator.RehydrateDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := item.Text, "beta"; got != want {
		t.Fatalf("rehydrated text = %q, want %q", got, want)
	}
	if got, want := item.Trace.Decision, RetentionActionRetainSource; got != want {
		t.Fatalf("trace decision = %q, want %q", got, want)
	}
}

func TestContextPackMaterializesRehydratableDecisions(t *testing.T) {
	text := stringsJoinLines(
		"& [Example] is source",
		"  = must: preserve exact text",
		"  ? [Question] is unresolved",
	)
	document := SourceDocument{Path: "example.sop", Digest: SourceDigest(text), Text: text}
	annotations, err := SOPReferenceProducer{}.ProduceReferences(document)
	if err != nil {
		t.Fatal(err)
	}
	simulator, err := NewRetentionSimulator(DefaultRetentionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := simulator.Simulate(RetentionRequest{
		References:      annotations.References(),
		CurrentPosition: 200,
	})
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

	pack, err := rehydrator.MaterializeContextPack(decisions)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack) == 0 {
		t.Fatal("expected rehydrated context pack items")
	}
	if err := pack.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, want := pack[0].Text, "& [Example] is source"; got != want {
		t.Fatalf("first pack item text = %q, want %q", got, want)
	}
	if len(pack.StableKeys()) != len(pack) {
		t.Fatal("expected stable key for every pack item")
	}
}

func TestMemorySourceStoreValidatesSHA256Digest(t *testing.T) {
	_, err := NewMemorySourceStore(SourceDocument{
		Path:   "source.sop",
		Digest: "sha256:bad",
		Text:   "source",
	})
	if err == nil {
		t.Fatal("expected digest validation error")
	}
}

func TestRehydratorRejectsMissingSource(t *testing.T) {
	store, err := NewMemorySourceStore(SourceDocument{Path: "other.sop", Text: "other"})
	if err != nil {
		t.Fatal(err)
	}
	rehydrator, err := NewRehydrator(store)
	if err != nil {
		t.Fatal(err)
	}
	ref := Reference{
		Address: ReferenceAddress{
			Layer:  ReferenceLayerSurface,
			Source: &SourceAddress{Path: "missing.sop", BeginLine: 1, EndLine: 1},
		},
	}
	decision := retentionDecision(ref, ref.StableKey(), ReferenceLifecycleRehydratable, RetentionActionRetainSource, "source preserved")

	if _, err := rehydrator.RehydrateDecision(decision); err == nil {
		t.Fatal("expected missing source error")
	}
}
