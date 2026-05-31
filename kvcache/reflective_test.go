package kvcache

import "testing"

func TestReflectiveReferenceValidation(t *testing.T) {
	span := TokenSpan{Sequence: 0, Begin: 12, End: 20}
	param := ParameterAddress{Layer: 17, Module: "attention.q_proj", Tensor: "weight", Offset: 2331, Size: 128}
	activation := ActivationAddress{Layer: 17, Head: 7, Begin: 64, End: 128}

	tests := []struct {
		name string
		ref  Reference
	}{
		{
			name: "surface token span",
			ref: Reference{
				Address: ReferenceAddress{Layer: ReferenceLayerSurface, Token: &span},
			},
		},
		{
			name: "subtext anchor linked to source span",
			ref: Reference{
				Address:    ReferenceAddress{Layer: ReferenceLayerSubtext, Anchor: "subtext:tone:uncertain", Token: &span},
				Role:       "subtext",
				Confidence: 0.64,
			},
		},
		{
			name: "contract anchor",
			ref: Reference{
				Address:    ReferenceAddress{Layer: ReferenceLayerContract, Anchor: "contract:Specification_Governance"},
				Role:       "obligation",
				Confidence: 1,
			},
		},
		{
			name: "reflective parameter address",
			ref: Reference{
				Address:    ReferenceAddress{Layer: ReferenceLayerReflective, Anchor: "transformer:weight", Parameter: &param},
				Role:       "parameter",
				Confidence: 1,
			},
		},
		{
			name: "reflective activation address",
			ref: Reference{
				Address:    ReferenceAddress{Layer: ReferenceLayerReflective, Anchor: "transformer:activation", Activation: &activation},
				Role:       "activation",
				Confidence: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ref.Validate(); err != nil {
				t.Fatal(err)
			}
			if tt.ref.StableKey() == "" {
				t.Fatal("expected stable key")
			}
		})
	}
}

func TestReflectiveReferenceValidationRejectsInvalidReferences(t *testing.T) {
	span := TokenSpan{Sequence: 0, Begin: 12, End: 12}
	param := ParameterAddress{Layer: 17, Module: "attention.q_proj", Tensor: "weight", Offset: 2331}
	activation := ActivationAddress{Layer: 17, Head: 7, Begin: 64, End: 64}

	tests := []struct {
		name string
		ref  Reference
	}{
		{name: "unknown layer", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayer("unknown"), Anchor: "x"}}},
		{name: "empty target", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerSubtext}}},
		{name: "surface without token", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Anchor: "surface"}}},
		{name: "contract without anchor", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerContract, Token: &TokenSpan{End: 1}}}},
		{name: "reflective without transformer address", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, Anchor: "reflective"}}},
		{name: "invalid token span", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Token: &span}}},
		{name: "invalid parameter address", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, Parameter: &param}}},
		{name: "invalid activation address", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, Activation: &activation}}},
		{name: "invalid confidence", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerSubtext, Anchor: "subtext"}, Confidence: 1.1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ref.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestReflectiveReferenceSetStableKeysPreserveOrder(t *testing.T) {
	span := TokenSpan{Sequence: 1, Begin: 4, End: 9}
	param := ParameterAddress{Layer: 3, Module: "mlp", Tensor: "gate.weight", Offset: 1024, Size: 256}
	refs := ReferenceSet{
		{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Token: &span}},
		{Address: ReferenceAddress{Layer: ReferenceLayerReflective, Parameter: &param}, Role: "parameter"},
	}

	if err := refs.Validate(); err != nil {
		t.Fatal(err)
	}

	keys := refs.StableKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0] == keys[1] {
		t.Fatal("expected distinct keys")
	}
	if got, want := keys[0], "layer=surface|seq=1:tok=4-9"; got != want {
		t.Fatalf("unexpected first key:\ngot:  %s\nwant: %s", got, want)
	}
}
