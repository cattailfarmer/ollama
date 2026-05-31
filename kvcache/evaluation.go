package kvcache

import "fmt"

type EvaluationClaimStatus string

const (
	EvaluationClaimStatusNoClaim EvaluationClaimStatus = "no_claim"
)

func (s EvaluationClaimStatus) Valid() bool {
	return s == EvaluationClaimStatusNoClaim
}

type ExpectedAnchor struct {
	Anchor string
	Layer  ReferenceLayer
	Role   string
}

func (a ExpectedAnchor) Validate() error {
	if a.Anchor == "" {
		return fmt.Errorf("kvcache: expected anchor is required")
	}
	if a.Layer != "" && !a.Layer.Valid() {
		return fmt.Errorf("kvcache: invalid expected anchor layer: %q", a.Layer)
	}

	return nil
}

type RehydrationExpectation struct {
	Anchor       string
	ContainsText string
}

func (e RehydrationExpectation) Validate() error {
	if e.Anchor == "" {
		return fmt.Errorf("kvcache: rehydration expectation anchor is required")
	}
	if e.ContainsText == "" {
		return fmt.Errorf("kvcache: rehydration expectation text is required")
	}

	return nil
}

type EvaluationTaskCase struct {
	Name                    string
	Document                SourceDocument
	ExpectedAnchors         []ExpectedAnchor
	RehydrationExpectations []RehydrationExpectation
	CurrentPosition         int32
	RuntimePolicy           RuntimePolicyConfig
}

func (c EvaluationTaskCase) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("kvcache: evaluation task name is required")
	}
	if err := c.Document.Validate(); err != nil {
		return err
	}
	if c.CurrentPosition < 0 {
		return fmt.Errorf("kvcache: current position must be non-negative: %d", c.CurrentPosition)
	}
	if err := c.RuntimePolicy.Validate(); err != nil {
		return err
	}
	if c.RuntimePolicy.Mode.normalized() == RuntimePolicyModeGuardedApply {
		return fmt.Errorf("kvcache: evaluation harness does not run guarded_apply mode")
	}
	for i, anchor := range c.ExpectedAnchors {
		if err := anchor.Validate(); err != nil {
			return fmt.Errorf("kvcache: expected anchor %d: %w", i, err)
		}
	}
	for i, expectation := range c.RehydrationExpectations {
		if err := expectation.Validate(); err != nil {
			return fmt.Errorf("kvcache: rehydration expectation %d: %w", i, err)
		}
	}

	return nil
}

type EvaluationScoreRecord struct {
	Name        string
	Passed      bool
	Observed    int
	Expected    int
	Description string
}

func (r EvaluationScoreRecord) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("kvcache: evaluation score name is required")
	}
	if r.Description == "" {
		return fmt.Errorf("kvcache: evaluation score description is required")
	}
	if r.Observed < 0 || r.Expected < 0 {
		return fmt.Errorf("kvcache: evaluation score counts must be non-negative: %d/%d", r.Observed, r.Expected)
	}

	return nil
}

type EvaluationScoreSet []EvaluationScoreRecord

func (s EvaluationScoreSet) Validate() error {
	for i, score := range s {
		if err := score.Validate(); err != nil {
			return fmt.Errorf("kvcache: evaluation score %d: %w", i, err)
		}
	}

	return nil
}

func (s EvaluationScoreSet) Passed() int {
	count := 0
	for _, score := range s {
		if score.Passed {
			count++
		}
	}

	return count
}

type EvaluationRecord struct {
	TaskName           string
	ClaimStatus        EvaluationClaimStatus
	Annotations        ReferenceAnnotations
	PolicyResult       RuntimePolicyResult
	MissingAnchors     []ExpectedAnchor
	MissingRehydration []RehydrationExpectation
	Scores             EvaluationScoreSet
}

func (r EvaluationRecord) Validate() error {
	if r.TaskName == "" {
		return fmt.Errorf("kvcache: evaluation record task name is required")
	}
	if !r.ClaimStatus.Valid() {
		return fmt.Errorf("kvcache: invalid evaluation claim status: %q", r.ClaimStatus)
	}
	if err := r.Annotations.Validate(); err != nil {
		return err
	}
	if err := r.PolicyResult.Validate(); err != nil {
		return err
	}
	if err := r.Scores.Validate(); err != nil {
		return err
	}

	return nil
}

func (r EvaluationRecord) Passed() bool {
	for _, score := range r.Scores {
		if !score.Passed {
			return false
		}
	}

	return true
}

type EvaluationHarness struct {
	Producer ReferenceProducer
}

func NewEvaluationHarness() EvaluationHarness {
	return EvaluationHarness{Producer: SOPReferenceProducer{}}
}

func (h EvaluationHarness) Evaluate(task EvaluationTaskCase) (EvaluationRecord, error) {
	if h.Producer == nil {
		h.Producer = SOPReferenceProducer{}
	}
	if err := task.Validate(); err != nil {
		return EvaluationRecord{}, err
	}

	annotations, err := h.Producer.ProduceReferences(task.Document)
	if err != nil {
		return EvaluationRecord{}, err
	}

	store, err := NewMemorySourceStore(task.Document)
	if err != nil {
		return EvaluationRecord{}, err
	}
	rehydrator, err := NewRehydrator(store)
	if err != nil {
		return EvaluationRecord{}, err
	}
	policy, err := NewRuntimePolicy(task.RuntimePolicy)
	if err != nil {
		return EvaluationRecord{}, err
	}
	policy.Rehydrator = rehydrator

	policyResult, err := policy.Apply(RuntimePolicyRequest{
		References:      annotations.References(),
		CurrentPosition: task.CurrentPosition,
	})
	if err != nil {
		return EvaluationRecord{}, err
	}

	missingAnchors := missingExpectedAnchors(task.ExpectedAnchors, annotations)
	missingRehydration := missingRehydrationExpectations(task.RehydrationExpectations, policyResult.ContextPack)
	scores := evaluationScores(task, annotations, policyResult, missingAnchors, missingRehydration)
	record := EvaluationRecord{
		TaskName:           task.Name,
		ClaimStatus:        EvaluationClaimStatusNoClaim,
		Annotations:        annotations,
		PolicyResult:       policyResult,
		MissingAnchors:     missingAnchors,
		MissingRehydration: missingRehydration,
		Scores:             scores,
	}
	if err := record.Validate(); err != nil {
		return EvaluationRecord{}, err
	}

	return record, nil
}

func missingExpectedAnchors(expected []ExpectedAnchor, annotations ReferenceAnnotations) []ExpectedAnchor {
	var missing []ExpectedAnchor
	for _, want := range expected {
		if !annotationsContainAnchor(annotations, want) {
			missing = append(missing, want)
		}
	}

	return missing
}

func annotationsContainAnchor(annotations ReferenceAnnotations, expected ExpectedAnchor) bool {
	for _, annotation := range annotations {
		ref := annotation.Reference
		if ref.Address.Anchor != expected.Anchor {
			continue
		}
		if expected.Layer != "" && ref.Address.Layer != expected.Layer {
			continue
		}
		if expected.Role != "" && ref.Role != expected.Role {
			continue
		}

		return true
	}

	return false
}

func missingRehydrationExpectations(expected []RehydrationExpectation, pack ContextPack) []RehydrationExpectation {
	var missing []RehydrationExpectation
	for _, want := range expected {
		if !contextPackContains(pack, want) {
			missing = append(missing, want)
		}
	}

	return missing
}

func contextPackContains(pack ContextPack, expected RehydrationExpectation) bool {
	for _, item := range pack {
		if item.Reference.Address.Anchor == expected.Anchor && contains(item.Text, expected.ContainsText) {
			return true
		}
	}

	return false
}

func evaluationScores(task EvaluationTaskCase, annotations ReferenceAnnotations, result RuntimePolicyResult, missingAnchors []ExpectedAnchor, missingRehydration []RehydrationExpectation) EvaluationScoreSet {
	return EvaluationScoreSet{
		{
			Name:        "anchors",
			Passed:      len(missingAnchors) == 0,
			Observed:    len(task.ExpectedAnchors) - len(missingAnchors),
			Expected:    len(task.ExpectedAnchors),
			Description: "expected typed anchors were produced by the fixture annotator",
		},
		{
			Name:        "policy_traces",
			Passed:      policyTraceExpectationPassed(task.RuntimePolicy.Mode.normalized(), result),
			Observed:    len(result.Traces),
			Expected:    expectedTraceCount(task.RuntimePolicy.Mode.normalized(), annotations),
			Description: "runtime policy mode produced fixture traces without benchmark or quality claims",
		},
		{
			Name:        "rehydration",
			Passed:      len(missingRehydration) == 0,
			Observed:    len(task.RehydrationExpectations) - len(missingRehydration),
			Expected:    len(task.RehydrationExpectations),
			Description: "expected context-pack text was rehydrated from fixture source spans",
		},
		{
			Name:        "claim_status",
			Passed:      true,
			Observed:    1,
			Expected:    1,
			Description: "evaluation record is explicitly marked no_claim",
		},
	}
}

func policyTraceExpectationPassed(mode RuntimePolicyMode, result RuntimePolicyResult) bool {
	switch mode {
	case RuntimePolicyModeOff:
		return len(result.Traces) == 0 && !result.Mutated
	case RuntimePolicyModeTraceOnly, RuntimePolicyModeSimulateOnly:
		return len(result.Traces) == len(result.Decisions) && !result.Mutated
	default:
		return false
	}
}

func expectedTraceCount(mode RuntimePolicyMode, annotations ReferenceAnnotations) int {
	switch mode {
	case RuntimePolicyModeTraceOnly, RuntimePolicyModeSimulateOnly:
		return len(annotations)
	default:
		return 0
	}
}

func contains(text, part string) bool {
	if part == "" {
		return true
	}
	for i := 0; i+len(part) <= len(text); i++ {
		if text[i:i+len(part)] == part {
			return true
		}
	}

	return false
}
