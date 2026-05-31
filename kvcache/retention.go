package kvcache

import (
	"fmt"
	"strconv"
)

type ReferenceLifecycle string

const (
	ReferenceLifecycleObserved     ReferenceLifecycle = "observed"
	ReferenceLifecycleValidated    ReferenceLifecycle = "validated"
	ReferenceLifecycleMaterialized ReferenceLifecycle = "materialized"
	ReferenceLifecycleCompressed   ReferenceLifecycle = "compressed"
	ReferenceLifecycleEvicted      ReferenceLifecycle = "evicted"
	ReferenceLifecycleRehydratable ReferenceLifecycle = "rehydratable"
	ReferenceLifecycleStale        ReferenceLifecycle = "stale"
	ReferenceLifecyclePinned       ReferenceLifecycle = "pinned"
)

func (l ReferenceLifecycle) Valid() bool {
	switch l {
	case ReferenceLifecycleObserved,
		ReferenceLifecycleValidated,
		ReferenceLifecycleMaterialized,
		ReferenceLifecycleCompressed,
		ReferenceLifecycleEvicted,
		ReferenceLifecycleRehydratable,
		ReferenceLifecycleStale,
		ReferenceLifecyclePinned:
		return true
	default:
		return false
	}
}

type RetentionAction string

const (
	RetentionActionRetainRawKV         RetentionAction = "retain_raw_kv"
	RetentionActionRetainSource        RetentionAction = "retain_source"
	RetentionActionCompressToReference RetentionAction = "compress_to_reference"
	RetentionActionRehydrateSpan       RetentionAction = "rehydrate_span"
	RetentionActionPinContract         RetentionAction = "pin_contract"
	RetentionActionMarkStale           RetentionAction = "mark_stale"
	RetentionActionAnnotate            RetentionAction = "annotate"
)

func (a RetentionAction) Valid() bool {
	switch a {
	case RetentionActionRetainRawKV,
		RetentionActionRetainSource,
		RetentionActionCompressToReference,
		RetentionActionRehydrateSpan,
		RetentionActionPinContract,
		RetentionActionMarkStale,
		RetentionActionAnnotate:
		return true
	default:
		return false
	}
}

type RetentionRequest struct {
	References      ReferenceSet
	CurrentPosition int32
}

func (r RetentionRequest) Validate() error {
	if r.CurrentPosition < 0 {
		return fmt.Errorf("kvcache: current position must be non-negative: %d", r.CurrentPosition)
	}
	if err := r.References.Validate(); err != nil {
		return err
	}

	return nil
}

type RetentionDecision struct {
	Reference Reference
	StableKey string
	Lifecycle ReferenceLifecycle
	Action    RetentionAction
	Reason    string
}

func (d RetentionDecision) Validate() error {
	if err := d.Reference.Validate(); err != nil {
		return err
	}
	if d.StableKey == "" {
		return fmt.Errorf("kvcache: retention decision stable key is required")
	}
	if !d.Lifecycle.Valid() {
		return fmt.Errorf("kvcache: invalid retention lifecycle: %q", d.Lifecycle)
	}
	if !d.Action.Valid() {
		return fmt.Errorf("kvcache: invalid retention action: %q", d.Action)
	}
	if d.Reason == "" {
		return fmt.Errorf("kvcache: retention decision reason is required")
	}

	return nil
}

type RetentionDecisionSet []RetentionDecision

func (ds RetentionDecisionSet) Validate() error {
	for i, decision := range ds {
		if err := decision.Validate(); err != nil {
			return fmt.Errorf("kvcache: retention decision %d: %w", i, err)
		}
	}

	return nil
}

func (ds RetentionDecisionSet) StableKeys() []string {
	keys := make([]string, len(ds))
	for i, decision := range ds {
		keys[i] = decision.StableKey
	}

	return keys
}

type BasicRetentionPolicy struct {
	RecentTokenWindow int32
	PinContracts      bool
	PinReflective     bool
	PreserveSources   bool
}

func DefaultRetentionPolicy() BasicRetentionPolicy {
	return BasicRetentionPolicy{
		RecentTokenWindow: 128,
		PinContracts:      true,
		PinReflective:     false,
		PreserveSources:   true,
	}
}

func (p BasicRetentionPolicy) Validate() error {
	if p.RecentTokenWindow < 0 {
		return fmt.Errorf("kvcache: recent token window must be non-negative: %d", p.RecentTokenWindow)
	}

	return nil
}

type RetentionSimulator struct {
	Policy BasicRetentionPolicy
}

func NewRetentionSimulator(policy BasicRetentionPolicy) (*RetentionSimulator, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}

	return &RetentionSimulator{Policy: policy}, nil
}

func (s *RetentionSimulator) Simulate(request RetentionRequest) (RetentionDecisionSet, error) {
	if s == nil {
		return nil, fmt.Errorf("kvcache: retention simulator is nil")
	}
	if err := s.Policy.Validate(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}

	decisions := make(RetentionDecisionSet, 0, len(request.References))
	for _, ref := range request.References {
		decision := s.decide(ref, request.CurrentPosition)
		if err := decision.Validate(); err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}

	return decisions, nil
}

func (s *RetentionSimulator) decide(ref Reference, currentPosition int32) RetentionDecision {
	address := ref.Address
	stableKey := ref.StableKey()

	if address.Layer == ReferenceLayerContract && s.Policy.PinContracts {
		return retentionDecision(ref, stableKey, ReferenceLifecyclePinned, RetentionActionPinContract, "contract reference is pinned as durable obligation")
	}

	if address.Layer == ReferenceLayerReflective {
		if s.Policy.PinReflective {
			return retentionDecision(ref, stableKey, ReferenceLifecyclePinned, RetentionActionAnnotate, "reflective reference is pinned for instrumentation")
		}

		return retentionDecision(ref, stableKey, ReferenceLifecycleCompressed, RetentionActionAnnotate, "reflective reference is retained as annotation without live cache mutation")
	}

	if address.Token != nil && withinRecentWindow(*address.Token, currentPosition, s.Policy.RecentTokenWindow) {
		return retentionDecision(ref, stableKey, ReferenceLifecycleMaterialized, RetentionActionRetainRawKV, "token span remains inside recent raw KV window")
	}

	if address.Source != nil && s.Policy.PreserveSources {
		return retentionDecision(ref, stableKey, ReferenceLifecycleRehydratable, RetentionActionRetainSource, "source address preserves exact rehydration path")
	}

	switch address.Layer {
	case ReferenceLayerSubtext, ReferenceLayerStructural:
		return retentionDecision(ref, stableKey, ReferenceLifecycleCompressed, RetentionActionCompressToReference, "interpretive layer can be represented as typed reference")
	case ReferenceLayerSurface:
		return retentionDecision(ref, stableKey, ReferenceLifecycleCompressed, RetentionActionCompressToReference, "surface span is outside recent window and has no source rehydration address")
	default:
		return retentionDecision(ref, stableKey, ReferenceLifecycleValidated, RetentionActionAnnotate, "reference validated without retention side effect")
	}
}

func retentionDecision(ref Reference, stableKey string, lifecycle ReferenceLifecycle, action RetentionAction, reason string) RetentionDecision {
	return RetentionDecision{
		Reference: ref,
		StableKey: stableKey,
		Lifecycle: lifecycle,
		Action:    action,
		Reason:    reason,
	}
}

func withinRecentWindow(span TokenSpan, currentPosition, window int32) bool {
	if window == 0 {
		return false
	}
	if span.End > currentPosition {
		return true
	}

	return currentPosition-span.End <= window
}

func (d RetentionDecision) StableKeyPart() string {
	return "decision:" + stableKeyField("reference", d.StableKey) +
		":" + stableKeyField("lifecycle", string(d.Lifecycle)) +
		":" + stableKeyField("action", string(d.Action)) +
		":" + stableKeyField("reason_len", strconv.Itoa(len(d.Reason)))
}
