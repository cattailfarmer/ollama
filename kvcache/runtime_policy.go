package kvcache

import (
	"fmt"
	"strings"
)

type RuntimePolicyMode string

const (
	RuntimePolicyModeOff          RuntimePolicyMode = "off"
	RuntimePolicyModeTraceOnly    RuntimePolicyMode = "trace_only"
	RuntimePolicyModeSimulateOnly RuntimePolicyMode = "simulate_only"
	RuntimePolicyModeGuardedApply RuntimePolicyMode = "guarded_apply"
)

func (m RuntimePolicyMode) Valid() bool {
	switch m.normalized() {
	case RuntimePolicyModeOff,
		RuntimePolicyModeTraceOnly,
		RuntimePolicyModeSimulateOnly,
		RuntimePolicyModeGuardedApply:
		return true
	default:
		return false
	}
}

func (m RuntimePolicyMode) normalized() RuntimePolicyMode {
	if m == "" {
		return RuntimePolicyModeOff
	}

	return m
}

type RuntimePolicyConfig struct {
	Mode            RuntimePolicyMode
	RetentionPolicy BasicRetentionPolicy
}

func DefaultRuntimePolicyConfig() RuntimePolicyConfig {
	return RuntimePolicyConfig{
		Mode:            RuntimePolicyModeOff,
		RetentionPolicy: DefaultRetentionPolicy(),
	}
}

func (c RuntimePolicyConfig) Validate() error {
	if !c.Mode.Valid() {
		return fmt.Errorf("kvcache: invalid runtime policy mode: %q", c.Mode)
	}
	if err := c.RetentionPolicy.Validate(); err != nil {
		return err
	}

	return nil
}

type RuntimePolicyRequest struct {
	References      ReferenceSet
	CurrentPosition int32
}

func (r RuntimePolicyRequest) Validate() error {
	if r.CurrentPosition < 0 {
		return fmt.Errorf("kvcache: current position must be non-negative: %d", r.CurrentPosition)
	}
	if err := r.References.Validate(); err != nil {
		return err
	}

	return nil
}

type RuntimePolicyFallback string

const (
	RuntimePolicyFallbackNone        RuntimePolicyFallback = ""
	RuntimePolicyFallbackPreserveRaw RuntimePolicyFallback = "preserve_raw"
)

type RuntimePolicyTrace struct {
	Mode            RuntimePolicyMode
	StableKey       string
	Lifecycle       ReferenceLifecycle
	CandidateAction RetentionAction
	FinalAction     RetentionAction
	Reason          string
	Fallback        RuntimePolicyFallback
	Resolved        bool
	Rehydrated      bool
	Mutated         bool
	Error           string
}

func (t RuntimePolicyTrace) Validate() error {
	if !t.Mode.Valid() || t.Mode.normalized() == RuntimePolicyModeOff {
		return fmt.Errorf("kvcache: runtime policy trace mode must be active: %q", t.Mode)
	}
	if t.StableKey == "" {
		return fmt.Errorf("kvcache: runtime policy trace stable key is required")
	}
	if !t.Lifecycle.Valid() {
		return fmt.Errorf("kvcache: invalid runtime policy trace lifecycle: %q", t.Lifecycle)
	}
	if !t.CandidateAction.Valid() {
		return fmt.Errorf("kvcache: invalid runtime policy candidate action: %q", t.CandidateAction)
	}
	if !t.FinalAction.Valid() {
		return fmt.Errorf("kvcache: invalid runtime policy final action: %q", t.FinalAction)
	}
	if t.Reason == "" {
		return fmt.Errorf("kvcache: runtime policy trace reason is required")
	}

	return nil
}

type RuntimePolicyTraceSet []RuntimePolicyTrace

func (ts RuntimePolicyTraceSet) Validate() error {
	for i, trace := range ts {
		if err := trace.Validate(); err != nil {
			return fmt.Errorf("kvcache: runtime policy trace %d: %w", i, err)
		}
	}

	return nil
}

type RuntimePolicyApplication struct {
	Decision   RetentionDecision
	Context    ContextPackItem
	HasContext bool
}

func (a RuntimePolicyApplication) Validate() error {
	if err := a.Decision.Validate(); err != nil {
		return err
	}
	if a.HasContext {
		if err := a.Context.Validate(); err != nil {
			return err
		}
	}

	return nil
}

type RuntimePolicyState interface {
	ApplyRuntimePolicy(application RuntimePolicyApplication) error
}

type MemoryRuntimePolicyState struct {
	applications []RuntimePolicyApplication
}

func NewMemoryRuntimePolicyState() *MemoryRuntimePolicyState {
	return &MemoryRuntimePolicyState{}
}

func (s *MemoryRuntimePolicyState) ApplyRuntimePolicy(application RuntimePolicyApplication) error {
	if s == nil {
		return fmt.Errorf("kvcache: runtime policy state is nil")
	}
	if err := application.Validate(); err != nil {
		return err
	}

	s.applications = append(s.applications, application)
	return nil
}

func (s *MemoryRuntimePolicyState) Applications() []RuntimePolicyApplication {
	if s == nil {
		return nil
	}

	applications := make([]RuntimePolicyApplication, len(s.applications))
	copy(applications, s.applications)
	return applications
}

type RuntimePolicyResult struct {
	Mode        RuntimePolicyMode
	Decisions   RetentionDecisionSet
	ContextPack ContextPack
	Traces      RuntimePolicyTraceSet
	Mutated     bool
	RolledBack  bool
}

func (r RuntimePolicyResult) Validate() error {
	if !r.Mode.Valid() {
		return fmt.Errorf("kvcache: invalid runtime policy result mode: %q", r.Mode)
	}
	if err := r.Decisions.Validate(); err != nil {
		return err
	}
	if err := r.ContextPack.Validate(); err != nil {
		return err
	}
	if err := r.Traces.Validate(); err != nil {
		return err
	}

	return nil
}

type RuntimePolicy struct {
	Config     RuntimePolicyConfig
	Resolver   ReferenceResolver
	Rehydrator *Rehydrator
	State      RuntimePolicyState
}

func NewRuntimePolicy(config RuntimePolicyConfig) (*RuntimePolicy, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	config.Mode = config.Mode.normalized()
	return &RuntimePolicy{Config: config}, nil
}

func (p *RuntimePolicy) Apply(request RuntimePolicyRequest) (RuntimePolicyResult, error) {
	if p == nil {
		return RuntimePolicyResult{}, fmt.Errorf("kvcache: runtime policy is nil")
	}

	mode := p.Config.Mode.normalized()
	result := RuntimePolicyResult{Mode: mode}
	if mode == RuntimePolicyModeOff {
		return result, nil
	}

	if err := p.Config.Validate(); err != nil {
		return RuntimePolicyResult{}, err
	}
	if err := request.Validate(); err != nil {
		return RuntimePolicyResult{}, err
	}

	simulator, err := NewRetentionSimulator(p.Config.RetentionPolicy)
	if err != nil {
		return RuntimePolicyResult{}, err
	}
	decisions, err := simulator.Simulate(RetentionRequest{
		References:      request.References,
		CurrentPosition: request.CurrentPosition,
	})
	if err != nil {
		return RuntimePolicyResult{}, err
	}

	result.Decisions = decisions
	for _, decision := range decisions {
		trace, item, hasContext := p.evaluateDecision(mode, decision)
		result.Traces = append(result.Traces, trace)
		if hasContext {
			result.ContextPack = append(result.ContextPack, item)
		}
		if trace.Fallback != RuntimePolicyFallbackNone {
			result.RolledBack = true
		}

		if mode != RuntimePolicyModeGuardedApply || trace.Fallback != RuntimePolicyFallbackNone {
			continue
		}

		application := RuntimePolicyApplication{
			Decision:   decision,
			Context:    item,
			HasContext: hasContext,
		}
		if p.State == nil {
			result.Traces[len(result.Traces)-1] = trace.withFallback("runtime policy state is unavailable")
			result.RolledBack = true
			continue
		}
		if err := p.State.ApplyRuntimePolicy(application); err != nil {
			result.Traces[len(result.Traces)-1] = trace.withFallback(err.Error())
			result.RolledBack = true
			continue
		}

		result.Traces[len(result.Traces)-1].Mutated = true
		result.Mutated = true
	}

	if err := result.Validate(); err != nil {
		return RuntimePolicyResult{}, err
	}

	return result, nil
}

func (p *RuntimePolicy) evaluateDecision(mode RuntimePolicyMode, decision RetentionDecision) (RuntimePolicyTrace, ContextPackItem, bool) {
	trace := RuntimePolicyTrace{
		Mode:            mode,
		StableKey:       decision.StableKey,
		Lifecycle:       decision.Lifecycle,
		CandidateAction: decision.Action,
		FinalAction:     decision.Action,
		Reason:          decision.Reason,
	}

	if mode == RuntimePolicyModeTraceOnly {
		return trace, ContextPackItem{}, false
	}

	if mode == RuntimePolicyModeGuardedApply && p.Resolver == nil {
		return trace.withFallback("reference resolver is unavailable"), ContextPackItem{}, false
	}
	if p.Resolver != nil {
		resolved, err := p.Resolver.ResolveReference(ResolveRequest{
			Reference: decision.Reference,
			Operation: resolveOperationForRetentionAction(decision.Action),
		})
		if err != nil {
			return trace.withFallback(err.Error()), ContextPackItem{}, false
		}
		if !resolved.Found {
			return trace.withFallback("reference resolution failed"), ContextPackItem{}, false
		}
		trace.Resolved = true
	}

	if !decisionRehydratable(decision) {
		return trace, ContextPackItem{}, false
	}
	if p.Rehydrator == nil {
		return trace.withFallback("rehydrator is unavailable"), ContextPackItem{}, false
	}

	item, err := p.Rehydrator.RehydrateDecision(decision)
	if err != nil {
		return trace.withFallback(err.Error()), ContextPackItem{}, false
	}

	trace.Rehydrated = true
	return trace, item, true
}

func (t RuntimePolicyTrace) withFallback(message string) RuntimePolicyTrace {
	t.FinalAction = RetentionActionRetainRawKV
	t.Fallback = RuntimePolicyFallbackPreserveRaw
	t.Error = appendTraceError(t.Error, message)
	t.Mutated = false

	return t
}

func appendTraceError(existing, message string) string {
	if existing == "" {
		return message
	}
	if message == "" {
		return existing
	}

	return existing + "; " + message
}

func resolveOperationForRetentionAction(action RetentionAction) ResolveOperation {
	switch action {
	case RetentionActionRetainRawKV:
		return ResolveOperationRetrieve
	case RetentionActionRetainSource, RetentionActionRehydrateSpan:
		return ResolveOperationRehydrate
	case RetentionActionPinContract:
		return ResolveOperationPin
	case RetentionActionCompressToReference, RetentionActionAnnotate:
		return ResolveOperationAnnotate
	case RetentionActionMarkStale:
		return ResolveOperationGate
	default:
		return ResolveOperationInspect
	}
}

func (m RuntimePolicyMode) String() string {
	return string(m.normalized())
}

func (f RuntimePolicyFallback) String() string {
	if f == RuntimePolicyFallbackNone {
		return ""
	}

	return string(f)
}

func (r RuntimePolicyResult) TraceSummary() string {
	parts := make([]string, len(r.Traces))
	for i, trace := range r.Traces {
		parts[i] = trace.StableKey + ":" + string(trace.FinalAction) + ":" + trace.Fallback.String()
	}

	return strings.Join(parts, "\n")
}
