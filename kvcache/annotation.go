package kvcache

import (
	"fmt"
	"strconv"
	"strings"
)

type AnnotationStatus string

const (
	AnnotationStatusCurrent AnnotationStatus = "current"
	AnnotationStatusStale   AnnotationStatus = "stale"
)

func (s AnnotationStatus) Valid() bool {
	switch s {
	case "", AnnotationStatusCurrent, AnnotationStatusStale:
		return true
	default:
		return false
	}
}

type SourceDocument struct {
	Path    string
	Digest  string
	Section string
	Text    string
}

func (d SourceDocument) Validate() error {
	if d.Path == "" && d.Digest == "" {
		return fmt.Errorf("kvcache: source document requires path or digest")
	}
	if d.Text == "" {
		return fmt.Errorf("kvcache: source document text is required")
	}

	return nil
}

type ReferenceAnnotation struct {
	Reference Reference
	Status    AnnotationStatus
}

func (a ReferenceAnnotation) Validate() error {
	if err := a.Reference.Validate(); err != nil {
		return err
	}
	if !a.Status.Valid() {
		return fmt.Errorf("kvcache: invalid annotation status: %q", a.Status)
	}

	return nil
}

func (a ReferenceAnnotation) IsStale() bool {
	return a.Status == AnnotationStatusStale
}

type ReferenceAnnotations []ReferenceAnnotation

func (as ReferenceAnnotations) Validate() error {
	for i, annotation := range as {
		if err := annotation.Validate(); err != nil {
			return fmt.Errorf("kvcache: annotation %d: %w", i, err)
		}
	}

	return nil
}

func (as ReferenceAnnotations) References() ReferenceSet {
	refs := make(ReferenceSet, len(as))
	for i, annotation := range as {
		refs[i] = annotation.Reference
	}

	return refs
}

func (as ReferenceAnnotations) MarkStale() ReferenceAnnotations {
	marked := make(ReferenceAnnotations, len(as))
	for i, annotation := range as {
		annotation.Status = AnnotationStatusStale
		marked[i] = annotation
	}

	return marked
}

type ReferenceProducer interface {
	ProduceReferences(document SourceDocument) (ReferenceAnnotations, error)
}

type SOPReferenceProducer struct{}

func (SOPReferenceProducer) ProduceReferences(document SourceDocument) (ReferenceAnnotations, error) {
	if err := document.Validate(); err != nil {
		return nil, err
	}

	var annotations ReferenceAnnotations
	walkSourceLines(document.Text, func(lineNo int, begin, end int64, line string) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return
		}

		source := SourceAddress{
			Path:      document.Path,
			Digest:    document.Digest,
			Section:   document.Section,
			BeginLine: lineNo,
			EndLine:   lineNo,
			BeginByte: begin,
			EndByte:   end,
		}

		annotations = append(annotations, referenceAnnotation(ReferenceLayerSurface, "surface", "line:"+strconv.Itoa(lineNo), 1, source))

		if layer, role, ok := classifySOPLine(trimmed); ok {
			anchor := annotationAnchor(role, lineNo, trimmed)
			confidence := float32(1)
			if layer == ReferenceLayerSubtext {
				confidence = 0.6
			}
			annotations = append(annotations, referenceAnnotation(layer, role, anchor, confidence, source))
		}
	})

	if err := annotations.Validate(); err != nil {
		return nil, err
	}

	return annotations, nil
}

func referenceAnnotation(layer ReferenceLayer, role, anchor string, confidence float32, source SourceAddress) ReferenceAnnotation {
	return ReferenceAnnotation{
		Reference: Reference{
			Address: ReferenceAddress{
				Layer:  layer,
				Anchor: anchor,
				Source: &source,
			},
			Role:       role,
			Confidence: confidence,
		},
		Status: AnnotationStatusCurrent,
	}
}

func classifySOPLine(line string) (ReferenceLayer, string, bool) {
	if line == "" {
		return "", "", false
	}

	switch line[0] {
	case '&', '+', '|', '@', '/', '*':
		return ReferenceLayerStructural, "structural", true
	case '=', '-':
		return ReferenceLayerContract, "contract", true
	case '?', '~':
		return ReferenceLayerSubtext, "subtext", true
	default:
		return "", "", false
	}
}

func annotationAnchor(role string, lineNo int, line string) string {
	if name, ok := bracketedName(line); ok {
		return role + ":" + name
	}

	return role + ":line:" + strconv.Itoa(lineNo)
}

func bracketedName(line string) (string, bool) {
	start := strings.IndexByte(line, '[')
	end := strings.IndexByte(line, ']')
	if start < 0 || end <= start+1 {
		return "", false
	}

	return line[start+1 : end], true
}

func walkSourceLines(text string, visit func(lineNo int, begin, end int64, line string)) {
	lineNo := 1
	begin := int64(0)

	for len(text) > 0 {
		next := strings.IndexByte(text, '\n')
		var segment string
		if next < 0 {
			segment = text
			text = ""
		} else {
			segment = text[:next+1]
			text = text[next+1:]
		}

		line := strings.TrimRight(segment, "\r\n")
		end := begin + int64(len(line))
		visit(lineNo, begin, end, line)
		begin += int64(len(segment))
		lineNo++
	}
}
