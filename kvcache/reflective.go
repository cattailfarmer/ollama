package kvcache

import (
	"fmt"
	"strconv"
	"strings"
)

type ReferenceLayer string

const (
	ReferenceLayerSurface    ReferenceLayer = "surface"
	ReferenceLayerSubtext    ReferenceLayer = "subtext"
	ReferenceLayerContract   ReferenceLayer = "contract"
	ReferenceLayerStructural ReferenceLayer = "structural"
	ReferenceLayerReflective ReferenceLayer = "reflective"
)

func (l ReferenceLayer) Valid() bool {
	switch l {
	case ReferenceLayerSurface,
		ReferenceLayerSubtext,
		ReferenceLayerContract,
		ReferenceLayerStructural,
		ReferenceLayerReflective:
		return true
	default:
		return false
	}
}

type TokenSpan struct {
	Sequence int
	Begin    int32
	End      int32
}

func (s TokenSpan) Validate() error {
	if s.Sequence < 0 {
		return fmt.Errorf("kvcache: token span sequence must be non-negative: %d", s.Sequence)
	}
	if s.Begin < 0 {
		return fmt.Errorf("kvcache: token span begin must be non-negative: %d", s.Begin)
	}
	if s.End <= s.Begin {
		return fmt.Errorf("kvcache: token span end must be greater than begin: %d <= %d", s.End, s.Begin)
	}

	return nil
}

func (s TokenSpan) stableKeyPart() string {
	return "seq=" + strconv.Itoa(s.Sequence) + ":tok=" + strconv.FormatInt(int64(s.Begin), 10) + "-" + strconv.FormatInt(int64(s.End), 10)
}

type ParameterAddress struct {
	Layer  int
	Module string
	Tensor string
	Offset int64
	Size   int64
}

func (a ParameterAddress) Validate() error {
	if a.Layer < 0 {
		return fmt.Errorf("kvcache: parameter layer must be non-negative: %d", a.Layer)
	}
	if a.Module == "" {
		return fmt.Errorf("kvcache: parameter module is required")
	}
	if a.Tensor == "" {
		return fmt.Errorf("kvcache: parameter tensor is required")
	}
	if a.Offset < 0 {
		return fmt.Errorf("kvcache: parameter offset must be non-negative: %d", a.Offset)
	}
	if a.Size <= 0 {
		return fmt.Errorf("kvcache: parameter size must be positive: %d", a.Size)
	}

	return nil
}

func (a ParameterAddress) stableKeyPart() string {
	return strings.Join([]string{
		"param",
		"layer=" + strconv.Itoa(a.Layer),
		"module=" + a.Module,
		"tensor=" + a.Tensor,
		"offset=" + strconv.FormatInt(a.Offset, 10),
		"size=" + strconv.FormatInt(a.Size, 10),
	}, ":")
}

type ActivationAddress struct {
	Layer int
	Head  int
	Begin int
	End   int
}

func (a ActivationAddress) Validate() error {
	if a.Layer < 0 {
		return fmt.Errorf("kvcache: activation layer must be non-negative: %d", a.Layer)
	}
	if a.Head < 0 {
		return fmt.Errorf("kvcache: activation head must be non-negative: %d", a.Head)
	}
	if a.Begin < 0 {
		return fmt.Errorf("kvcache: activation begin must be non-negative: %d", a.Begin)
	}
	if a.End <= a.Begin {
		return fmt.Errorf("kvcache: activation end must be greater than begin: %d <= %d", a.End, a.Begin)
	}

	return nil
}

func (a ActivationAddress) stableKeyPart() string {
	return strings.Join([]string{
		"activation",
		"layer=" + strconv.Itoa(a.Layer),
		"head=" + strconv.Itoa(a.Head),
		"channels=" + strconv.Itoa(a.Begin) + "-" + strconv.Itoa(a.End),
	}, ":")
}

type ReferenceAddress struct {
	Layer      ReferenceLayer
	Anchor     string
	Token      *TokenSpan
	Parameter  *ParameterAddress
	Activation *ActivationAddress
}

func (a ReferenceAddress) Validate() error {
	if !a.Layer.Valid() {
		return fmt.Errorf("kvcache: invalid reference layer: %q", a.Layer)
	}

	targets := 0
	if a.Anchor != "" {
		targets++
	}
	if a.Token != nil {
		if err := a.Token.Validate(); err != nil {
			return err
		}
		targets++
	}
	if a.Parameter != nil {
		if err := a.Parameter.Validate(); err != nil {
			return err
		}
		targets++
	}
	if a.Activation != nil {
		if err := a.Activation.Validate(); err != nil {
			return err
		}
		targets++
	}
	if targets == 0 {
		return fmt.Errorf("kvcache: reference address must identify at least one target")
	}

	switch a.Layer {
	case ReferenceLayerSurface:
		if a.Token == nil {
			return fmt.Errorf("kvcache: surface reference requires a token span")
		}
	case ReferenceLayerContract:
		if a.Anchor == "" {
			return fmt.Errorf("kvcache: contract reference requires an anchor")
		}
	case ReferenceLayerReflective:
		if a.Parameter == nil && a.Activation == nil {
			return fmt.Errorf("kvcache: reflective reference requires a parameter or activation address")
		}
	}

	return nil
}

func (a ReferenceAddress) StableKey() string {
	parts := []string{"layer=" + string(a.Layer)}
	if a.Anchor != "" {
		parts = append(parts, "anchor="+a.Anchor)
	}
	if a.Token != nil {
		parts = append(parts, a.Token.stableKeyPart())
	}
	if a.Parameter != nil {
		parts = append(parts, a.Parameter.stableKeyPart())
	}
	if a.Activation != nil {
		parts = append(parts, a.Activation.stableKeyPart())
	}

	return strings.Join(parts, "|")
}

type Reference struct {
	Address    ReferenceAddress
	Role       string
	Confidence float32
}

func (r Reference) Validate() error {
	if err := r.Address.Validate(); err != nil {
		return err
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("kvcache: confidence must be between 0 and 1: %f", r.Confidence)
	}

	return nil
}

func (r Reference) StableKey() string {
	if r.Role == "" {
		return r.Address.StableKey()
	}

	return r.Address.StableKey() + "|role=" + r.Role
}

type ReferenceSet []Reference

func (rs ReferenceSet) Validate() error {
	for i, ref := range rs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("kvcache: reference %d: %w", i, err)
		}
	}

	return nil
}

func (rs ReferenceSet) StableKeys() []string {
	keys := make([]string, len(rs))
	for i, ref := range rs {
		keys[i] = ref.StableKey()
	}

	return keys
}
