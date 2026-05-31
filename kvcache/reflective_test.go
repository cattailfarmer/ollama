package kvcache

import "testing"

func TestReflectiveReferenceValidation(t *testing.T) {
	span := TokenSpan{Sequence: 0, Begin: 12, End: 20}
	addressSpace := AddressSpace{
		Model:   ModelFingerprint{Architecture: "llama", Family: "qwen2", Tokenizer: "bpe", Digest: "sha256:abc"},
		Name:    "llama.cpp",
		Version: "1",
	}
	param := ParameterAddress{Layer: 17, Module: "attention.q_proj", Tensor: "weight", Offset: 2331, Size: 128}
	activation := ActivationAddress{Layer: 17, Head: 7, Begin: 64, End: 128}
	cacheCell := CacheCellAddress{Cache: "causal", Layer: 17, Sequence: 2, Position: 32, Cell: 4}
	source := SourceAddress{Path: "docs/reflective_kv_cache.sop", Digest: "sha256:def", Section: "ReflectiveKVCache", BeginLine: 1, EndLine: 12}

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
				Address:    ReferenceAddress{Layer: ReferenceLayerSubtext, Anchor: "subtext:tone:uncertain", Token: &span, Source: &source},
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
				Address:    ReferenceAddress{Layer: ReferenceLayerReflective, AddressSpace: &addressSpace, Anchor: "transformer:weight", Parameter: &param},
				Role:       "parameter",
				Confidence: 1,
			},
		},
		{
			name: "reflective activation address",
			ref: Reference{
				Address:    ReferenceAddress{Layer: ReferenceLayerReflective, AddressSpace: &addressSpace, Anchor: "transformer:activation", Activation: &activation},
				Role:       "activation",
				Confidence: 1,
			},
		},
		{
			name: "reflective cache cell address",
			ref: Reference{
				Address:    ReferenceAddress{Layer: ReferenceLayerReflective, AddressSpace: &addressSpace, Anchor: "cache:cell", CacheCell: &cacheCell},
				Role:       "cache_cell",
				Confidence: 1,
			},
		},
		{
			name: "surface source address",
			ref: Reference{
				Address:    ReferenceAddress{Layer: ReferenceLayerSurface, Source: &source},
				Role:       "source",
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
	addressSpace := AddressSpace{Model: ModelFingerprint{Architecture: "llama"}, Name: "llama.cpp", Version: "1"}
	param := ParameterAddress{Layer: 17, Module: "attention.q_proj", Tensor: "weight", Offset: 2331}
	activation := ActivationAddress{Layer: 17, Head: 7, Begin: 64, End: 64}
	cacheCell := CacheCellAddress{Cache: "causal", Layer: 0, Sequence: 0, Position: 0, Cell: -1}
	source := SourceAddress{Path: "docs/reflective_kv_cache.sop", BeginLine: 12, EndLine: 1}
	validParam := ParameterAddress{Layer: 17, Module: "attention.q_proj", Tensor: "weight", Offset: 2331, Size: 1}
	validActivation := ActivationAddress{Layer: 17, Head: 7, Begin: 64, End: 65}

	tests := []struct {
		name string
		ref  Reference
	}{
		{name: "unknown layer", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayer("unknown"), Anchor: "x"}}},
		{name: "empty target", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerSubtext}}},
		{name: "surface without token", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Anchor: "surface"}}},
		{name: "contract without anchor", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerContract, Token: &TokenSpan{End: 1}}}},
		{name: "reflective without transformer address", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, Anchor: "reflective"}}},
		{name: "reflective without address space", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, Parameter: &validParam}}},
		{name: "invalid token span", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Token: &span}}},
		{name: "invalid parameter address", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, Parameter: &param}}},
		{name: "invalid activation address", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, AddressSpace: &addressSpace, Activation: &activation}}},
		{name: "invalid cache cell address", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, AddressSpace: &addressSpace, CacheCell: &cacheCell}}},
		{name: "invalid source address", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Source: &source}}},
		{name: "invalid address space", ref: Reference{Address: ReferenceAddress{Layer: ReferenceLayerReflective, AddressSpace: &AddressSpace{Name: "llama.cpp", Version: "1"}, Activation: &validActivation}}},
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
	addressSpace := AddressSpace{Model: ModelFingerprint{Architecture: "llama"}, Name: "llama.cpp", Version: "1"}
	param := ParameterAddress{Layer: 3, Module: "mlp", Tensor: "gate.weight", Offset: 1024, Size: 256}
	refs := ReferenceSet{
		{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Token: &span}},
		{Address: ReferenceAddress{Layer: ReferenceLayerReflective, AddressSpace: &addressSpace, Parameter: &param}, Role: "parameter"},
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

func TestReflectiveStableKeysEscapeReservedCharacters(t *testing.T) {
	ref := Reference{
		Address: ReferenceAddress{
			Layer:  ReferenceLayerContract,
			Anchor: "contract:Specification|Governance=active",
		},
		Role:       "obligation:primary",
		Confidence: 1,
	}

	if err := ref.Validate(); err != nil {
		t.Fatal(err)
	}

	if got, want := ref.StableKey(), "layer=contract|anchor=contract%3ASpecification%7CGovernance%3Dactive|role=obligation%3Aprimary"; got != want {
		t.Fatalf("unexpected stable key:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestMemoryReferenceStorePreservesInsertionOrder(t *testing.T) {
	store := NewMemoryReferenceStore()
	first := Reference{Address: ReferenceAddress{Layer: ReferenceLayerContract, Anchor: "contract:Specification_Governance"}}
	second := Reference{Address: ReferenceAddress{Layer: ReferenceLayerSurface, Token: &TokenSpan{Sequence: 1, Begin: 4, End: 9}}}

	if err := store.PutReference(first); err != nil {
		t.Fatal(err)
	}
	if err := store.PutReference(second); err != nil {
		t.Fatal(err)
	}
	if err := store.PutReference(first); err != nil {
		t.Fatal(err)
	}

	refs := store.ListReferences()
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if got, want := refs[0].StableKey(), first.StableKey(); got != want {
		t.Fatalf("unexpected first key:\ngot:  %s\nwant: %s", got, want)
	}
	if got, want := refs[1].StableKey(), second.StableKey(); got != want {
		t.Fatalf("unexpected second key:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestMemoryReferenceStoreResolvesStoredReferences(t *testing.T) {
	store := NewMemoryReferenceStore()
	ref := Reference{
		Address:    ReferenceAddress{Layer: ReferenceLayerContract, Anchor: "contract:Specification_Governance"},
		Role:       "obligation",
		Confidence: 1,
	}

	if err := store.PutReference(ref); err != nil {
		t.Fatal(err)
	}

	resolved, err := store.ResolveReference(ResolveRequest{
		Reference: ref,
		Operation: ResolveOperationInspect,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Found {
		t.Fatal("expected stored reference to resolve")
	}
	if got, want := resolved.StableKey, ref.StableKey(); got != want {
		t.Fatalf("unexpected stable key:\ngot:  %s\nwant: %s", got, want)
	}
}
