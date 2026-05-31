package kvcache

import (
	"fmt"
	"strconv"
	"strings"
)

var stableKeyReplacer = strings.NewReplacer(
	"%", "%25",
	"|", "%7C",
	":", "%3A",
	"=", "%3D",
)

func stableKeyField(name, value string) string {
	return name + "=" + stableKeyReplacer.Replace(value)
}

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

type ModelFingerprint struct {
	Architecture string
	Family       string
	Tokenizer    string
	Digest       string
}

func (f ModelFingerprint) Validate() error {
	if f.Architecture == "" && f.Family == "" && f.Tokenizer == "" && f.Digest == "" {
		return fmt.Errorf("kvcache: model fingerprint must identify at least one model property")
	}

	return nil
}

func (f ModelFingerprint) stableKeyPart() string {
	parts := []string{"model"}
	if f.Architecture != "" {
		parts = append(parts, stableKeyField("arch", f.Architecture))
	}
	if f.Family != "" {
		parts = append(parts, stableKeyField("family", f.Family))
	}
	if f.Tokenizer != "" {
		parts = append(parts, stableKeyField("tokenizer", f.Tokenizer))
	}
	if f.Digest != "" {
		parts = append(parts, stableKeyField("digest", f.Digest))
	}

	return strings.Join(parts, ":")
}

type AddressSpace struct {
	Model   ModelFingerprint
	Name    string
	Version string
}

func (s AddressSpace) Validate() error {
	if err := s.Model.Validate(); err != nil {
		return err
	}
	if s.Name == "" {
		return fmt.Errorf("kvcache: address space name is required")
	}
	if s.Version == "" {
		return fmt.Errorf("kvcache: address space version is required")
	}

	return nil
}

func (s AddressSpace) stableKeyPart() string {
	return strings.Join([]string{
		"address_space",
		stableKeyField("name", s.Name),
		stableKeyField("version", s.Version),
		s.Model.stableKeyPart(),
	}, ":")
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
		stableKeyField("module", a.Module),
		stableKeyField("tensor", a.Tensor),
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

type CacheCellAddress struct {
	Cache    string
	Layer    int
	Sequence int
	Position int32
	Cell     int
}

func (a CacheCellAddress) Validate() error {
	if a.Cache == "" {
		return fmt.Errorf("kvcache: cache cell cache is required")
	}
	if a.Layer < 0 {
		return fmt.Errorf("kvcache: cache cell layer must be non-negative: %d", a.Layer)
	}
	if a.Sequence < 0 {
		return fmt.Errorf("kvcache: cache cell sequence must be non-negative: %d", a.Sequence)
	}
	if a.Position < 0 {
		return fmt.Errorf("kvcache: cache cell position must be non-negative: %d", a.Position)
	}
	if a.Cell < 0 {
		return fmt.Errorf("kvcache: cache cell index must be non-negative: %d", a.Cell)
	}

	return nil
}

func (a CacheCellAddress) stableKeyPart() string {
	return strings.Join([]string{
		"cache_cell",
		stableKeyField("cache", a.Cache),
		"layer=" + strconv.Itoa(a.Layer),
		"seq=" + strconv.Itoa(a.Sequence),
		"pos=" + strconv.FormatInt(int64(a.Position), 10),
		"cell=" + strconv.Itoa(a.Cell),
	}, ":")
}

type SourceAddress struct {
	Path      string
	Digest    string
	Section   string
	BeginLine int
	EndLine   int
	BeginByte int64
	EndByte   int64
}

func (a SourceAddress) Validate() error {
	if a.Path == "" && a.Digest == "" {
		return fmt.Errorf("kvcache: source address requires path or digest")
	}
	if a.BeginLine < 0 || a.EndLine < 0 {
		return fmt.Errorf("kvcache: source address lines must be non-negative: %d-%d", a.BeginLine, a.EndLine)
	}
	if a.BeginLine > 0 && a.EndLine > 0 && a.EndLine < a.BeginLine {
		return fmt.Errorf("kvcache: source address end line must be greater than or equal to begin line: %d < %d", a.EndLine, a.BeginLine)
	}
	if a.BeginByte < 0 || a.EndByte < 0 {
		return fmt.Errorf("kvcache: source address byte offsets must be non-negative: %d-%d", a.BeginByte, a.EndByte)
	}
	if a.BeginByte > 0 && a.EndByte > 0 && a.EndByte <= a.BeginByte {
		return fmt.Errorf("kvcache: source address end byte must be greater than begin byte: %d <= %d", a.EndByte, a.BeginByte)
	}

	return nil
}

func (a SourceAddress) stableKeyPart() string {
	parts := []string{"source"}
	if a.Path != "" {
		parts = append(parts, stableKeyField("path", a.Path))
	}
	if a.Digest != "" {
		parts = append(parts, stableKeyField("digest", a.Digest))
	}
	if a.Section != "" {
		parts = append(parts, stableKeyField("section", a.Section))
	}
	if a.BeginLine > 0 || a.EndLine > 0 {
		parts = append(parts, "lines="+strconv.Itoa(a.BeginLine)+"-"+strconv.Itoa(a.EndLine))
	}
	if a.BeginByte > 0 || a.EndByte > 0 {
		parts = append(parts, "bytes="+strconv.FormatInt(a.BeginByte, 10)+"-"+strconv.FormatInt(a.EndByte, 10))
	}

	return strings.Join(parts, ":")
}

type ReferenceAddress struct {
	Layer        ReferenceLayer
	AddressSpace *AddressSpace
	Anchor       string
	Token        *TokenSpan
	Parameter    *ParameterAddress
	Activation   *ActivationAddress
	CacheCell    *CacheCellAddress
	Source       *SourceAddress
}

func (a ReferenceAddress) Validate() error {
	if !a.Layer.Valid() {
		return fmt.Errorf("kvcache: invalid reference layer: %q", a.Layer)
	}

	targets := 0
	if a.Anchor != "" {
		targets++
	}
	if a.AddressSpace != nil {
		if err := a.AddressSpace.Validate(); err != nil {
			return err
		}
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
	if a.CacheCell != nil {
		if err := a.CacheCell.Validate(); err != nil {
			return err
		}
		targets++
	}
	if a.Source != nil {
		if err := a.Source.Validate(); err != nil {
			return err
		}
		targets++
	}
	if targets == 0 {
		return fmt.Errorf("kvcache: reference address must identify at least one target")
	}

	switch a.Layer {
	case ReferenceLayerSurface:
		if a.Token == nil && a.Source == nil {
			return fmt.Errorf("kvcache: surface reference requires a token span or source address")
		}
	case ReferenceLayerContract:
		if a.Anchor == "" {
			return fmt.Errorf("kvcache: contract reference requires an anchor")
		}
	case ReferenceLayerReflective:
		if a.Parameter == nil && a.Activation == nil && a.CacheCell == nil {
			return fmt.Errorf("kvcache: reflective reference requires a parameter, activation, or cache cell address")
		}
		if a.AddressSpace == nil {
			return fmt.Errorf("kvcache: reflective reference requires an address space")
		}
	}

	return nil
}

func (a ReferenceAddress) StableKey() string {
	parts := []string{"layer=" + string(a.Layer)}
	if a.AddressSpace != nil {
		parts = append(parts, a.AddressSpace.stableKeyPart())
	}
	if a.Anchor != "" {
		parts = append(parts, stableKeyField("anchor", a.Anchor))
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
	if a.CacheCell != nil {
		parts = append(parts, a.CacheCell.stableKeyPart())
	}
	if a.Source != nil {
		parts = append(parts, a.Source.stableKeyPart())
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

	return r.Address.StableKey() + "|" + stableKeyField("role", r.Role)
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

type ResolveOperation string

const (
	ResolveOperationInspect   ResolveOperation = "inspect"
	ResolveOperationRetrieve  ResolveOperation = "retrieve"
	ResolveOperationPin       ResolveOperation = "pin"
	ResolveOperationEvict     ResolveOperation = "evict"
	ResolveOperationRehydrate ResolveOperation = "rehydrate"
	ResolveOperationGate      ResolveOperation = "gate"
	ResolveOperationAnnotate  ResolveOperation = "annotate"
)

func (o ResolveOperation) Valid() bool {
	switch o {
	case ResolveOperationInspect,
		ResolveOperationRetrieve,
		ResolveOperationPin,
		ResolveOperationEvict,
		ResolveOperationRehydrate,
		ResolveOperationGate,
		ResolveOperationAnnotate:
		return true
	default:
		return false
	}
}

type ResolveRequest struct {
	Reference Reference
	Operation ResolveOperation
}

func (r ResolveRequest) Validate() error {
	if err := r.Reference.Validate(); err != nil {
		return err
	}
	if !r.Operation.Valid() {
		return fmt.Errorf("kvcache: invalid resolve operation: %q", r.Operation)
	}

	return nil
}

type ResolvedReference struct {
	Reference Reference
	Operation ResolveOperation
	StableKey string
	Found     bool
}

type ReferenceResolver interface {
	ResolveReference(request ResolveRequest) (ResolvedReference, error)
}

type ReferenceStore interface {
	PutReference(reference Reference) error
	GetReference(stableKey string) (Reference, bool)
	DeleteReference(stableKey string)
	ListReferences() ReferenceSet
}

type MemoryReferenceStore struct {
	refs  map[string]Reference
	order []string
}

func NewMemoryReferenceStore() *MemoryReferenceStore {
	return &MemoryReferenceStore{
		refs: make(map[string]Reference),
	}
}

func (s *MemoryReferenceStore) PutReference(reference Reference) error {
	if err := reference.Validate(); err != nil {
		return err
	}

	key := reference.StableKey()
	if _, ok := s.refs[key]; !ok {
		s.order = append(s.order, key)
	}
	s.refs[key] = reference

	return nil
}

func (s *MemoryReferenceStore) GetReference(stableKey string) (Reference, bool) {
	ref, ok := s.refs[stableKey]
	return ref, ok
}

func (s *MemoryReferenceStore) DeleteReference(stableKey string) {
	if _, ok := s.refs[stableKey]; !ok {
		return
	}

	delete(s.refs, stableKey)
	for i, key := range s.order {
		if key == stableKey {
			s.order = append(s.order[:i], s.order[i+1:]...)
			return
		}
	}
}

func (s *MemoryReferenceStore) ListReferences() ReferenceSet {
	refs := make(ReferenceSet, 0, len(s.order))
	for _, key := range s.order {
		ref, ok := s.refs[key]
		if ok {
			refs = append(refs, ref)
		}
	}

	return refs
}

func (s *MemoryReferenceStore) ResolveReference(request ResolveRequest) (ResolvedReference, error) {
	if err := request.Validate(); err != nil {
		return ResolvedReference{}, err
	}

	stableKey := request.Reference.StableKey()
	ref, ok := s.GetReference(stableKey)
	if !ok {
		ref = request.Reference
	}

	return ResolvedReference{
		Reference: ref,
		Operation: request.Operation,
		StableKey: stableKey,
		Found:     ok,
	}, nil
}
