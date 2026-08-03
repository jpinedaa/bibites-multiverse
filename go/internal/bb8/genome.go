package bb8

// The canonical genome projection and genomeHash of
// contracts/genome-hash.md (`bb8-genome/1`).
//
// The projection keeps only what is inherited — the gene object, the brain's
// nodes, the brain's synapses and the game version — and serializes it in
// exactly one legal byte form, so two independent implementations on two
// machines produce the same 64 hex characters for the same organism (D11,
// m3_considerations.md Risk 9).
//
// The bytes are written directly rather than through encoding/json: §5.5 needs
// escaping rules Go's encoder does not have (it escapes <, > and & by default),
// and §5.4 needs floats written as the "f32:" bit pattern rather than as JSON
// numbers.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Projection is the projection identifier, written into the hashed bytes and
// into the hash label (genome-hash.md §6).
const Projection = "bb8-genome/1"

// HashPrefix is the label every genomeHash carries. The label is part of the
// value: a consumer compares whole strings and never truncates (§6).
const HashPrefix = Projection + ":sha256:"

// ErrUnhashableGenome is returned for every cause in genome-hash.md §8. There
// is never a partial hash, a placeholder, or the hash of a repaired projection.
var ErrUnhashableGenome = errors.New("bb8: genome cannot be hashed")

func unhashable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnhashableGenome, fmt.Sprintf(format, args...))
}

// nodeTypeNames is the ordinal table of NEATBrain.NodeType (genome-hash.md
// §4.1, NEATBrain.cs:73-87). An unknown ordinal is projected unchanged; an
// unknown name is unhashable, because guessing an ordinal for it would make two
// implementations disagree.
var nodeTypeNames = []string{
	"Input", "Sigmoid", "Linear", "TanH", "Sine", "ReLu", "Gaussian",
	"Latch", "Differential", "Abs", "Mult", "Integrator", "Inhibitory", "SoftLatch",
}

// GenomeHash returns the full labelled hash of one organism's genome.
//
// gameVersion is the caller's authoritative version (contract-a.md §4.6) and is
// used only when the blob carries no $.version.
func GenomeHash(payload, gameVersion string) (string, error) {
	canon, err := Canonical(payload, gameVersion)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canon))
	return HashPrefix + hex.EncodeToString(sum[:]), nil
}

// HashHex returns the 64 hex digits of a bb8-genome/1 hash, or "" when the
// label is not one this implementation produced. It exists for file-path
// sharding and log lines only — §6 forbids a truncated hash in any field, key
// or comparison.
func HashHex(hash string) string {
	if !strings.HasPrefix(hash, HashPrefix) {
		return ""
	}
	h := hash[len(HashPrefix):]
	if len(h) != 64 {
		return ""
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ""
		}
	}
	return h
}

// ValidGenomeHash reports whether h is a well-formed bb8-genome/1 value.
func ValidGenomeHash(h string) bool { return HashHex(h) != "" }

type projNode struct {
	baseActivation string
	desc           string
	index          int64
	inov           int64
	typ            int64
}

type projSynapse struct {
	en      bool
	inov    int64
	nodeIn  int64
	nodeOut int64
	weight  string
}

// Canonical returns the canonical UTF-8 byte string of genome-hash.md §5 — one
// line, no whitespace, no trailing newline.
func Canonical(payload, gameVersion string) (string, error) {
	if payload == "" {
		return "", unhashable("payload is empty")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &root); err != nil || root == nil {
		return "", unhashable("payload is not a JSON object")
	}

	// §3: the dialect is selected by the presence of the top-level body key,
	// exactly as SaveSystem.LoadBibiteOrEggFromData keys off egg.
	var genesRaw, nodesRaw, synapsesRaw json.RawMessage
	if _, saved := root["body"]; saved {
		wrapper, err := object(root["genes"])
		if err != nil {
			return "", unhashable("saved dialect: $.genes is not an object")
		}
		genesRaw = wrapper["genes"]
		brain, err := object(root["brain"])
		if err != nil {
			return "", unhashable("saved dialect: $.brain is not an object")
		}
		nodesRaw = brain["Nodes"]
		synapsesRaw = brain["Synapses"]
	} else {
		genesRaw = root["genes"]
		nodesRaw = root["nodes"]
		synapsesRaw = root["synapses"]
	}

	genes, err := object(genesRaw)
	if err != nil {
		return "", unhashable("no gene object at the dialect's path")
	}
	nodeItems, err := array(nodesRaw)
	if err != nil {
		return "", unhashable("no node array at the dialect's path")
	}
	synItems, err := array(synapsesRaw)
	if err != nil {
		// §3 makes only a missing gene object or node array unhashable. A blob
		// with no synapse array is a brain with no synapses.
		synItems = nil
	}

	nodes := make([]projNode, 0, len(nodeItems))
	for i, raw := range nodeItems {
		n, err := projectNode(raw)
		if err != nil {
			return "", fmt.Errorf("node %d: %w", i, err)
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(a, b int) bool { return nodes[a].index < nodes[b].index })
	for i := 1; i < len(nodes); i++ {
		if nodes[i].index == nodes[i-1].index {
			return "", unhashable("two nodes share index %d", nodes[i].index)
		}
	}

	synapses := make([]projSynapse, 0, len(synItems))
	for i, raw := range synItems {
		s, err := projectSynapse(raw)
		if err != nil {
			return "", fmt.Errorf("synapse %d: %w", i, err)
		}
		synapses = append(synapses, s)
	}
	sort.Slice(synapses, func(a, b int) bool {
		x, y := synapses[a], synapses[b]
		if x.nodeIn != y.nodeIn {
			return x.nodeIn < y.nodeIn
		}
		if x.nodeOut != y.nodeOut {
			return x.nodeOut < y.nodeOut
		}
		return x.inov < y.inov
	})
	for i := 1; i < len(synapses); i++ {
		x, y := synapses[i-1], synapses[i]
		if x.nodeIn == y.nodeIn && x.nodeOut == y.nodeOut && x.inov == y.inov {
			return "", unhashable("two synapses share (nodeIn %d, nodeOut %d, inov %d)",
				x.nodeIn, x.nodeOut, x.inov)
		}
	}

	// §3: the version tag is $.version in both dialects; the caller's
	// authoritative gameVersion fills in when the blob carries none. The
	// projection never has an empty version.
	version := gameVersion
	if raw, ok := root["version"]; ok && kindOf(raw) == '"' {
		v, err := decodeString(raw)
		if err != nil {
			return "", err
		}
		version = v
	}
	if version == "" {
		return "", unhashable("no $.version in the blob and no authoritative gameVersion")
	}

	geneNames := make([]string, 0, len(genes))
	for name := range genes {
		geneNames = append(geneNames, name)
	}
	// §5.2: ascending by the byte sequence of the UTF-8 encoding. Go compares
	// strings byte by byte, which is exactly that rule.
	sort.Strings(geneNames)

	var b bytes.Buffer
	b.WriteString(`{"genes":{`)
	for i, name := range geneNames {
		if i > 0 {
			b.WriteByte(',')
		}
		if !utf8.ValidString(name) {
			return "", unhashable("gene name is not valid UTF-8")
		}
		writeJSONString(&b, name)
		b.WriteByte(':')
		f, err := f32Token(genes[name])
		if err != nil {
			return "", fmt.Errorf("gene %q: %w", name, err)
		}
		writeJSONString(&b, f)
	}
	b.WriteString(`},"nodes":[`)
	for i, n := range nodes {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"baseActivation":`)
		writeJSONString(&b, n.baseActivation)
		b.WriteString(`,"desc":`)
		writeJSONString(&b, n.desc)
		b.WriteString(`,"index":`)
		b.WriteString(strconv.FormatInt(n.index, 10))
		b.WriteString(`,"inov":`)
		b.WriteString(strconv.FormatInt(n.inov, 10))
		b.WriteString(`,"type":`)
		b.WriteString(strconv.FormatInt(n.typ, 10))
		b.WriteByte('}')
	}
	b.WriteString(`],"projection":`)
	writeJSONString(&b, Projection)
	b.WriteString(`,"synapses":[`)
	for i, s := range synapses {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"en":`)
		if s.en {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString(`,"inov":`)
		b.WriteString(strconv.FormatInt(s.inov, 10))
		b.WriteString(`,"nodeIn":`)
		b.WriteString(strconv.FormatInt(s.nodeIn, 10))
		b.WriteString(`,"nodeOut":`)
		b.WriteString(strconv.FormatInt(s.nodeOut, 10))
		b.WriteString(`,"weight":`)
		writeJSONString(&b, s.weight)
		b.WriteByte('}')
	}
	b.WriteString(`],"version":`)
	writeJSONString(&b, version)
	b.WriteByte('}')
	return b.String(), nil
}

func projectNode(raw json.RawMessage) (projNode, error) {
	obj, err := object(raw)
	if err != nil {
		return projNode{}, unhashable("node is not an object")
	}
	var n projNode

	// §4.1: baseActivation defaults to 0.0, desc to "", inov to 0; a missing
	// Index or an unusable Type makes the genome unhashable.
	n.baseActivation = "f32:00000000"
	if v, ok := obj["baseActivation"]; ok && kindOf(v) != 'n' {
		s, err := f32Token(v)
		if err != nil {
			return n, fmt.Errorf("baseActivation: %w", err)
		}
		n.baseActivation = s
	}
	if v, ok := obj["Desc"]; ok && kindOf(v) != 'n' {
		s, err := decodeString(v)
		if err != nil {
			return n, fmt.Errorf("Desc: %w", err)
		}
		n.desc = s
	}
	idx, ok := obj["Index"]
	if !ok || kindOf(idx) == 'n' {
		return n, unhashable("node has no Index")
	}
	if n.index, err = intToken(idx); err != nil {
		return n, fmt.Errorf("Index: %w", err)
	}
	if v, ok := obj["Inov"]; ok && kindOf(v) != 'n' {
		if n.inov, err = intToken(v); err != nil {
			return n, fmt.Errorf("Inov: %w", err)
		}
	}
	typRaw, ok := obj["Type"]
	if !ok || kindOf(typRaw) == 'n' {
		typRaw, ok = obj["TypeName"]
		if !ok || kindOf(typRaw) == 'n' {
			return n, unhashable("node has neither Type nor TypeName")
		}
	}
	switch kindOf(typRaw) {
	case '"':
		name, err := decodeString(typRaw)
		if err != nil {
			return n, fmt.Errorf("Type: %w", err)
		}
		ordinal := -1
		for i, candidate := range nodeTypeNames {
			if candidate == name {
				ordinal = i
				break
			}
		}
		if ordinal < 0 {
			return n, unhashable("node type name %q is not in the NodeType table", name)
		}
		n.typ = int64(ordinal)
	case '0':
		if n.typ, err = intToken(typRaw); err != nil {
			return n, fmt.Errorf("Type: %w", err)
		}
	default:
		return n, unhashable("node Type is neither a number nor a name")
	}
	return n, nil
}

func projectSynapse(raw json.RawMessage) (projSynapse, error) {
	obj, err := object(raw)
	if err != nil {
		return projSynapse{}, unhashable("synapse is not an object")
	}
	// §4.2: En defaults to true. A disabled synapse stays in the projection —
	// it is inherited topology and CopyBrain carries it to the child.
	s := projSynapse{en: true}
	if v, ok := obj["En"]; ok && kindOf(v) != 'n' {
		if kindOf(v) != 't' {
			return s, unhashable("En is not a boolean")
		}
		s.en = strings.TrimSpace(string(v)) == "true"
	}
	if v, ok := obj["Inov"]; ok && kindOf(v) != 'n' {
		if s.inov, err = intToken(v); err != nil {
			return s, fmt.Errorf("Inov: %w", err)
		}
	}
	in, ok := obj["NodeIn"]
	if !ok || kindOf(in) == 'n' {
		return s, unhashable("synapse has no NodeIn")
	}
	if s.nodeIn, err = intToken(in); err != nil {
		return s, fmt.Errorf("NodeIn: %w", err)
	}
	out, ok := obj["NodeOut"]
	if !ok || kindOf(out) == 'n' {
		return s, unhashable("synapse has no NodeOut")
	}
	if s.nodeOut, err = intToken(out); err != nil {
		return s, fmt.Errorf("NodeOut: %w", err)
	}
	w, ok := obj["Weight"]
	if !ok || kindOf(w) == 'n' {
		return s, unhashable("synapse has no Weight")
	}
	// §4.2: Weight is polymorphic. A number becomes the f32: form; a gene
	// reference string becomes "ref:" + the string, verbatim after the prefix.
	switch kindOf(w) {
	case '"':
		ref, err := decodeString(w)
		if err != nil {
			return s, fmt.Errorf("Weight: %w", err)
		}
		s.weight = "ref:" + ref
	case '0':
		if s.weight, err = f32Token(w); err != nil {
			return s, fmt.Errorf("Weight: %w", err)
		}
	default:
		return s, unhashable("synapse Weight is neither a number nor a string")
	}
	return s, nil
}

// f32Token implements genome-hash.md §5.4: a correctly rounded decimal to
// binary32 parse, NaN/Inf/overflow rejected, -0.0 normalized to +0.0, and the
// raw 32 bits printed big-endian as eight lowercase hex digits.
func f32Token(raw json.RawMessage) (string, error) {
	s := strings.TrimSpace(string(raw))
	if !isJSONNumber(s) {
		return "", unhashable("%q is not a JSON number", s)
	}
	f, err := strconv.ParseFloat(s, 32)
	if err != nil {
		// strconv reports ErrRange for a finite literal that overflows
		// binary32, which §5.4 makes unhashable.
		return "", unhashable("%q is not a finite binary32 value", s)
	}
	v := float32(f)
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return "", unhashable("%q is NaN or Inf", s)
	}
	bits := math.Float32bits(v)
	if bits == 0x80000000 {
		bits = 0 // -0.0 and +0.0 both spell f32:00000000
	}
	return fmt.Sprintf("f32:%08x", bits), nil
}

// intToken implements the integer half of §5.4. A number written with a
// fractional part that is exactly integral is accepted; the range is int64.
func intToken(raw json.RawMessage) (int64, error) {
	s := strings.TrimSpace(string(raw))
	if !isJSONNumber(s) {
		return 0, unhashable("%q is not a JSON number", s)
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != math.Trunc(f) || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, unhashable("%q is not an integer", s)
	}
	if f < -9223372036854775808.0 || f >= 9223372036854775808.0 {
		return 0, unhashable("%q is outside int64", s)
	}
	return int64(f), nil
}

func isJSONNumber(s string) bool {
	if s == "" {
		return false
	}
	if s[0] != '-' && (s[0] < '0' || s[0] > '9') {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9', c == '-', c == '+', c == '.', c == 'e', c == 'E':
		default:
			return false
		}
	}
	return true
}

// decodeString decodes a JSON string and enforces §5.5's UTF-8 rule.
func decodeString(raw json.RawMessage) (string, error) {
	if kindOf(raw) != '"' {
		return "", unhashable("value is not a string")
	}
	if !utf8.Valid(raw) {
		return "", unhashable("string is not valid UTF-8")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", unhashable("string does not decode: %v", err)
	}
	if !utf8.ValidString(s) {
		return "", unhashable("string is not valid UTF-8")
	}
	return s, nil
}

// writeJSONString implements §5.5's minimal, deterministic escaping: quote and
// backslash, the C0 controls, and nothing else. In particular <, > and & are
// raw, and no non-ASCII character is ever escaped.
func writeJSONString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if c < 0x20 {
				fmt.Fprintf(b, `\u%04x`, c)
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}

func object(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if kindOf(raw) != '{' {
		return nil, errors.New("not an object")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, errors.New("not an object")
	}
	return m, nil
}

func array(raw json.RawMessage) ([]json.RawMessage, error) {
	if kindOf(raw) != '[' {
		return nil, errors.New("not an array")
	}
	var a []json.RawMessage
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errors.New("not an array")
	}
	return a, nil
}

// kindOf classifies a raw JSON value by its first significant byte.
func kindOf(raw json.RawMessage) byte {
	for _, c := range raw {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', '[', '"', 'n':
			return c
		case 't', 'f':
			return 't'
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return '0'
		default:
			return c
		}
	}
	return 0
}
