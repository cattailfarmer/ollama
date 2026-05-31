package kvcache

import "testing"

func TestEvaluationHarnessScoresFixtureWithoutClaims(t *testing.T) {
	text := stringsJoinLines(
		"& [Task] is evaluation fixture",
		"  = [Requirement] must preserve source evidence",
		"  ? [Question] is a scored anchor",
	)
	document := SourceDocument{Path: "eval.sop", Digest: SourceDigest(text), Text: text}
	harness := NewEvaluationHarness()

	record, err := harness.Evaluate(EvaluationTaskCase{
		Name:     "fixture anchors and rehydration",
		Document: document,
		ExpectedAnchors: []ExpectedAnchor{
			{Anchor: "structural:Task", Layer: ReferenceLayerStructural, Role: "structural"},
			{Anchor: "contract:Requirement", Layer: ReferenceLayerContract, Role: "contract"},
			{Anchor: "subtext:Question", Layer: ReferenceLayerSubtext, Role: "subtext"},
		},
		RehydrationExpectations: []RehydrationExpectation{
			{Anchor: "structural:Task", ContainsText: "& [Task] is evaluation fixture"},
		},
		CurrentPosition: 256,
		RuntimePolicy: RuntimePolicyConfig{
			Mode:            RuntimePolicyModeSimulateOnly,
			RetentionPolicy: DefaultRetentionPolicy(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	if record.ClaimStatus != EvaluationClaimStatusNoClaim {
		t.Fatalf("claim status = %q, want %q", record.ClaimStatus, EvaluationClaimStatusNoClaim)
	}
	if !record.Passed() {
		t.Fatalf("expected fixture scores to pass: %+v", record.Scores)
	}
	if len(record.PolicyResult.Traces) != len(record.Annotations) {
		t.Fatalf("trace count = %d, want annotation count %d", len(record.PolicyResult.Traces), len(record.Annotations))
	}
	if record.PolicyResult.Mutated {
		t.Fatal("evaluation harness must not mutate runtime state")
	}
	if len(record.PolicyResult.ContextPack) == 0 {
		t.Fatal("expected simulated policy to materialize rehydration evidence")
	}
}

func TestEvaluationHarnessDefaultOffPolicyIsNoOp(t *testing.T) {
	text := "& [Task] is default-off fixture"
	document := SourceDocument{Path: "off.sop", Digest: SourceDigest(text), Text: text}
	harness := NewEvaluationHarness()

	record, err := harness.Evaluate(EvaluationTaskCase{
		Name:     "default off",
		Document: document,
		ExpectedAnchors: []ExpectedAnchor{
			{Anchor: "structural:Task", Layer: ReferenceLayerStructural},
		},
		RuntimePolicy: DefaultRuntimePolicyConfig(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !record.Passed() {
		t.Fatalf("default-off fixture should pass without policy traces: %+v", record.Scores)
	}
	if len(record.PolicyResult.Traces) != 0 {
		t.Fatalf("default-off policy emitted %d traces", len(record.PolicyResult.Traces))
	}
	if len(record.PolicyResult.ContextPack) != 0 {
		t.Fatalf("default-off policy materialized %d context items", len(record.PolicyResult.ContextPack))
	}
}

func TestEvaluationHarnessReportsMissingAnchors(t *testing.T) {
	text := "& [Task] is missing-anchor fixture"
	document := SourceDocument{Path: "missing.sop", Digest: SourceDigest(text), Text: text}
	harness := NewEvaluationHarness()

	record, err := harness.Evaluate(EvaluationTaskCase{
		Name:     "missing anchor",
		Document: document,
		ExpectedAnchors: []ExpectedAnchor{
			{Anchor: "contract:Absent", Layer: ReferenceLayerContract},
		},
		RuntimePolicy: RuntimePolicyConfig{
			Mode:            RuntimePolicyModeTraceOnly,
			RetentionPolicy: DefaultRetentionPolicy(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Passed() {
		t.Fatalf("expected missing anchor to fail score: %+v", record.Scores)
	}
	if got, want := len(record.MissingAnchors), 1; got != want {
		t.Fatalf("missing anchors = %d, want %d", got, want)
	}
	if record.ClaimStatus != EvaluationClaimStatusNoClaim {
		t.Fatalf("missing-anchor record still must be no_claim, got %q", record.ClaimStatus)
	}
}

func TestEvaluationHarnessRejectsGuardedApplyMode(t *testing.T) {
	text := "& [Task] is guarded fixture"
	document := SourceDocument{Path: "guarded.sop", Digest: SourceDigest(text), Text: text}
	harness := NewEvaluationHarness()

	_, err := harness.Evaluate(EvaluationTaskCase{
		Name:     "guarded apply",
		Document: document,
		RuntimePolicy: RuntimePolicyConfig{
			Mode:            RuntimePolicyModeGuardedApply,
			RetentionPolicy: DefaultRetentionPolicy(),
		},
	})
	if err == nil {
		t.Fatal("expected guarded_apply to be rejected by evaluation harness")
	}
}
