package kvcache

import "testing"

func TestSOPReferenceProducerEmitsLayeredReferences(t *testing.T) {
	document := SourceDocument{
		Path:   "platform/refinement/example.sop",
		Digest: "sha256:abc",
		Text: stringsJoinLines(
			"& [Example] is a subject",
			"  + [field] is contained data",
			"  = must: preserve provenance",
			"  ? [uncertain] what follows",
			"ordinary prompt text",
		),
	}

	annotations, err := SOPReferenceProducer{}.ProduceReferences(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := annotations.Validate(); err != nil {
		t.Fatal(err)
	}

	layers := map[ReferenceLayer]bool{}
	for _, annotation := range annotations {
		layers[annotation.Reference.Address.Layer] = true
		if annotation.Reference.Address.Source == nil {
			t.Fatal("expected source provenance on every annotation")
		}
	}

	for _, layer := range []ReferenceLayer{
		ReferenceLayerSurface,
		ReferenceLayerStructural,
		ReferenceLayerContract,
		ReferenceLayerSubtext,
	} {
		if !layers[layer] {
			t.Fatalf("expected layer %q", layer)
		}
	}
}

func TestSOPReferenceProducerIsDeterministic(t *testing.T) {
	document := SourceDocument{
		Path: "prompt.sop",
		Text: stringsJoinLines(
			"& [Prompt] is source",
			"  - never: lose exact text",
		),
	}
	producer := SOPReferenceProducer{}

	first, err := producer.ProduceReferences(document)
	if err != nil {
		t.Fatal(err)
	}
	second, err := producer.ProduceReferences(document)
	if err != nil {
		t.Fatal(err)
	}

	firstKeys := first.References().StableKeys()
	secondKeys := second.References().StableKeys()
	if len(firstKeys) != len(secondKeys) {
		t.Fatalf("key count mismatch: %d != %d", len(firstKeys), len(secondKeys))
	}
	for i := range firstKeys {
		if firstKeys[i] != secondKeys[i] {
			t.Fatalf("key %d mismatch:\ngot:  %s\nwant: %s", i, secondKeys[i], firstKeys[i])
		}
	}
}

func TestSOPReferenceProducerMarksAnnotationsStale(t *testing.T) {
	annotations, err := SOPReferenceProducer{}.ProduceReferences(SourceDocument{
		Path: "prompt.sop",
		Text: "& [Prompt] is source",
	})
	if err != nil {
		t.Fatal(err)
	}

	stale := annotations.MarkStale()
	if len(stale) != len(annotations) {
		t.Fatalf("expected stale count %d, got %d", len(annotations), len(stale))
	}
	for _, annotation := range stale {
		if !annotation.IsStale() {
			t.Fatal("expected stale annotation")
		}
	}
	for _, annotation := range annotations {
		if annotation.IsStale() {
			t.Fatal("original annotations should remain current")
		}
	}
}

func TestSOPReferenceProducerRejectsInvalidDocuments(t *testing.T) {
	_, err := SOPReferenceProducer{}.ProduceReferences(SourceDocument{Text: "& [Prompt] is source"})
	if err == nil {
		t.Fatal("expected missing path or digest error")
	}

	_, err = SOPReferenceProducer{}.ProduceReferences(SourceDocument{Path: "prompt.sop"})
	if err == nil {
		t.Fatal("expected missing text error")
	}
}

func stringsJoinLines(lines ...string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}

	return out
}
