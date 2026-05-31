package kvcache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func SourceDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidateSourceDigest(document SourceDocument) error {
	if document.Digest == "" || !strings.HasPrefix(document.Digest, "sha256:") {
		return nil
	}

	if got, want := SourceDigest(document.Text), document.Digest; got != want {
		return fmt.Errorf("kvcache: source digest mismatch for %q: got %s, want %s", document.Path, got, want)
	}

	return nil
}

type SourceStore interface {
	GetSource(address SourceAddress) (SourceDocument, bool)
}

type MemorySourceStore struct {
	sources map[string]SourceDocument
}

func NewMemorySourceStore(documents ...SourceDocument) (*MemorySourceStore, error) {
	store := &MemorySourceStore{sources: make(map[string]SourceDocument)}
	for _, document := range documents {
		if err := store.PutSource(document); err != nil {
			return nil, err
		}
	}

	return store, nil
}

func (s *MemorySourceStore) PutSource(document SourceDocument) error {
	if s == nil {
		return fmt.Errorf("kvcache: source store is nil")
	}
	if err := document.Validate(); err != nil {
		return err
	}
	if err := ValidateSourceDigest(document); err != nil {
		return err
	}
	if s.sources == nil {
		s.sources = make(map[string]SourceDocument)
	}
	if document.Path != "" {
		s.sources["path:"+document.Path] = document
	}
	if document.Digest != "" {
		s.sources["digest:"+document.Digest] = document
	}

	return nil
}

func (s *MemorySourceStore) GetSource(address SourceAddress) (SourceDocument, bool) {
	if s == nil {
		return SourceDocument{}, false
	}
	if address.Digest != "" {
		if document, ok := s.sources["digest:"+address.Digest]; ok {
			return document, true
		}
	}
	if address.Path != "" {
		if document, ok := s.sources["path:"+address.Path]; ok {
			return document, true
		}
	}

	return SourceDocument{}, false
}

type RehydrationTrace struct {
	StableKey string
	Source    SourceAddress
	Decision  RetentionAction
}

func (t RehydrationTrace) Validate() error {
	if t.StableKey == "" {
		return fmt.Errorf("kvcache: rehydration trace stable key is required")
	}
	if err := t.Source.Validate(); err != nil {
		return err
	}
	if !t.Decision.Valid() {
		return fmt.Errorf("kvcache: invalid rehydration trace decision: %q", t.Decision)
	}

	return nil
}

type ContextPackItem struct {
	Reference Reference
	StableKey string
	Source    SourceAddress
	Text      string
	Trace     RehydrationTrace
}

func (i ContextPackItem) Validate() error {
	if err := i.Reference.Validate(); err != nil {
		return err
	}
	if i.StableKey == "" {
		return fmt.Errorf("kvcache: context pack item stable key is required")
	}
	if err := i.Source.Validate(); err != nil {
		return err
	}
	if i.Text == "" {
		return fmt.Errorf("kvcache: context pack item text is required")
	}
	if err := i.Trace.Validate(); err != nil {
		return err
	}

	return nil
}

type ContextPack []ContextPackItem

func (p ContextPack) Validate() error {
	for i, item := range p {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("kvcache: context pack item %d: %w", i, err)
		}
	}

	return nil
}

func (p ContextPack) Text() string {
	parts := make([]string, len(p))
	for i, item := range p {
		parts[i] = item.Text
	}

	return strings.Join(parts, "\n")
}

func (p ContextPack) StableKeys() []string {
	keys := make([]string, len(p))
	for i, item := range p {
		keys[i] = item.StableKey
	}

	return keys
}

type Rehydrator struct {
	Store SourceStore
}

func NewRehydrator(store SourceStore) (*Rehydrator, error) {
	if store == nil {
		return nil, fmt.Errorf("kvcache: source store is required")
	}

	return &Rehydrator{Store: store}, nil
}

func (r *Rehydrator) RehydrateDecision(decision RetentionDecision) (ContextPackItem, error) {
	if r == nil || r.Store == nil {
		return ContextPackItem{}, fmt.Errorf("kvcache: rehydrator requires a source store")
	}
	if err := decision.Validate(); err != nil {
		return ContextPackItem{}, err
	}
	if !decisionRehydratable(decision) {
		return ContextPackItem{}, fmt.Errorf("kvcache: retention decision is not rehydratable: %s", decision.StableKey)
	}
	if decision.Reference.Address.Source == nil {
		return ContextPackItem{}, fmt.Errorf("kvcache: rehydratable decision has no source address: %s", decision.StableKey)
	}

	source := *decision.Reference.Address.Source
	document, ok := r.Store.GetSource(source)
	if !ok {
		return ContextPackItem{}, fmt.Errorf("kvcache: source document not found for %s", decision.StableKey)
	}
	if err := ValidateSourceDigest(document); err != nil {
		return ContextPackItem{}, err
	}

	text, err := extractSourceText(document.Text, source)
	if err != nil {
		return ContextPackItem{}, err
	}

	item := ContextPackItem{
		Reference: decision.Reference,
		StableKey: decision.StableKey,
		Source:    source,
		Text:      text,
		Trace: RehydrationTrace{
			StableKey: decision.StableKey,
			Source:    source,
			Decision:  decision.Action,
		},
	}
	if err := item.Validate(); err != nil {
		return ContextPackItem{}, err
	}

	return item, nil
}

func (r *Rehydrator) MaterializeContextPack(decisions RetentionDecisionSet) (ContextPack, error) {
	if err := decisions.Validate(); err != nil {
		return nil, err
	}

	pack := make(ContextPack, 0, len(decisions))
	for _, decision := range decisions {
		if !decisionRehydratable(decision) {
			continue
		}

		item, err := r.RehydrateDecision(decision)
		if err != nil {
			return nil, err
		}
		pack = append(pack, item)
	}

	if err := pack.Validate(); err != nil {
		return nil, err
	}

	return pack, nil
}

func decisionRehydratable(decision RetentionDecision) bool {
	return decision.Lifecycle == ReferenceLifecycleRehydratable ||
		decision.Action == RetentionActionRetainSource ||
		decision.Action == RetentionActionRehydrateSpan
}

func extractSourceText(text string, address SourceAddress) (string, error) {
	if address.EndByte > address.BeginByte {
		if address.EndByte > int64(len(text)) {
			return "", fmt.Errorf("kvcache: source byte range exceeds document length: %d > %d", address.EndByte, len(text))
		}

		return text[address.BeginByte:address.EndByte], nil
	}

	if address.BeginLine <= 0 || address.EndLine <= 0 {
		return "", fmt.Errorf("kvcache: source address requires byte range or line range")
	}

	begin, end, ok := lineByteRange(text, address.BeginLine, address.EndLine)
	if !ok {
		return "", fmt.Errorf("kvcache: source line range not found: %d-%d", address.BeginLine, address.EndLine)
	}

	return text[begin:end], nil
}

func lineByteRange(text string, beginLine, endLine int) (int64, int64, bool) {
	if beginLine <= 0 || endLine < beginLine {
		return 0, 0, false
	}

	var begin, end int64
	foundBegin := false
	walkSourceLines(text, func(lineNo int, lineBegin, lineEnd int64, line string) {
		if lineNo == beginLine {
			begin = lineBegin
			foundBegin = true
		}
		if lineNo == endLine && foundBegin {
			end = lineEnd
		}
	})

	return begin, end, foundBegin && end >= begin
}
