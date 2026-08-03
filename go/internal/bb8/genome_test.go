package bb8

import (
	"errors"
	"strings"
	"testing"
)

// workedExample is contracts/genome-hash.md §7.1's input blob, verbatim.
const workedExample = `{
  "transform": { "position": [2000.0, 412.77], "rotation": 274.11, "scale": 0.9312 },
  "rb2d": { "px": 2000.0, "py": 412.77, "vx": 6.12, "vy": 0.44, "r": 274.11 },
  "genes": {
    "tag": "Cyan", "speciesID": 41, "isReady": true, "gen": 37,
    "parent1": -843827577,
    "genes": { "SizeRatio": 1.0, "ColorG": 0.5, "ColorB": 0.30000001192092896 }
  },
  "body": { "id": { "id": -1180911975 }, "health": 42.5 },
  "clock": { "tic": 9, "timeAlive": 812.5 },
  "brain": {
    "isReady": true,
    "Nodes": [
      { "Type": 3, "baseActivation": -0.25, "TypeName": "TanH", "Index": 48,
        "Inov": 117, "Desc": "Hidden1", "Value": 0.13, "LastInput": 0.9,
        "LastOutput": 0.13 },
      { "Type": 0, "baseActivation": 0.0, "TypeName": "Input", "Index": 0,
        "Inov": 0, "Desc": "Constant", "Value": 1.0, "LastInput": 0.0,
        "LastOutput": 1.0 }
    ],
    "Synapses": [
      { "Inov": 204, "NodeIn": 0, "NodeOut": 48, "Weight": -1.5, "En": true }
    ]
  },
  "version": "0.6.3.1",
  "desc": "a worked example"
}`

// workedCanonical is §7.1's canonical string — 390 bytes, one line, no
// trailing newline.
const workedCanonical = `{"genes":{"ColorB":"f32:3e99999a","ColorG":"f32:3f000000","SizeRatio":"f32:3f800000"},` +
	`"nodes":[{"baseActivation":"f32:00000000","desc":"Constant","index":0,"inov":0,"type":0},` +
	`{"baseActivation":"f32:be800000","desc":"Hidden1","index":48,"inov":117,"type":3}],` +
	`"projection":"bb8-genome/1","synapses":[{"en":true,"inov":204,"nodeIn":0,"nodeOut":48,` +
	`"weight":"f32:bfc00000"}],"version":"0.6.3.1"}`

const (
	workedHash = "bb8-genome/1:sha256:1725de8f1b61ba91fbeea7c91c47d3060b6ff97afbb6dfc2fc4062879a8bee14"
	mutantHash = "bb8-genome/1:sha256:43ea8315112301799622f3ab8906c2882e528502bc35424a209cd67bfdaec207"
)

// TestWorkedExampleReproducesTheSpec is conformance case 1: the canonical
// string is byte-for-byte what genome-hash.md §7.1 prints, and so is the digest.
func TestWorkedExampleReproducesTheSpec(t *testing.T) {
	canon, err := Canonical(workedExample, "")
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	if len(canon) != 390 {
		t.Errorf("canonical string is %d bytes, want the 390 the spec states", len(canon))
	}
	if canon != workedCanonical {
		t.Fatalf("canonical string differs from genome-hash.md §7.1:\n got %s\nwant %s", canon, workedCanonical)
	}
	if strings.Contains(canon, "\n") {
		t.Error("the canonical string must be one line with no trailing newline")
	}
	got, err := GenomeHash(workedExample, "")
	if err != nil {
		t.Fatalf("GenomeHash: %v", err)
	}
	if got != workedHash {
		t.Fatalf("genomeHash = %s, want %s", got, workedHash)
	}
}

// TestOneULPMutantDiffers is conformance case 4 and the second half of
// m3_considerations.md Risk 9: a mutated child never inherits its parent's hash.
func TestOneULPMutantDiffers(t *testing.T) {
	mutant := strings.Replace(workedExample, "0.30000001192092896", "0.3000001", 1)
	if mutant == workedExample {
		t.Fatal("the mutation was not applied")
	}
	got, err := GenomeHash(mutant, "")
	if err != nil {
		t.Fatalf("GenomeHash: %v", err)
	}
	if got == workedHash {
		t.Fatal("a one-ULP gene change produced the parent's digest")
	}
	if got != mutantHash {
		t.Fatalf("mutant genomeHash = %s, want %s", got, mutantHash)
	}
	canon, _ := Canonical(mutant, "")
	if !strings.Contains(canon, `"ColorB":"f32:3e99999d"`) {
		t.Fatalf("mutant canonical string does not carry f32:3e99999d: %s", canon)
	}
}

// TestConformanceChecklist walks the nine cases of genome-hash.md §7.4.
func TestConformanceChecklist(t *testing.T) {
	// Case 2: the same organism with the two nodes and the gene keys reordered.
	reordered := `{
	  "body": { "id": -1180911975 },
	  "genes": { "gen": 37, "genes": { "SizeRatio": 1.0, "ColorB": 0.30000001192092896, "ColorG": 0.5 } },
	  "brain": {
	    "Nodes": [
	      { "Type": 0, "baseActivation": 0.0, "TypeName": "Input", "Index": 0, "Inov": 0, "Desc": "Constant" },
	      { "Type": 3, "baseActivation": -0.25, "TypeName": "TanH", "Index": 48, "Inov": 117, "Desc": "Hidden1" }
	    ],
	    "Synapses": [ { "Inov": 204, "NodeIn": 0, "NodeOut": 48, "Weight": -1.5, "En": true } ]
	  },
	  "version": "0.6.3.1"
	}`

	// Case 3: the template dialect of the same organism. No body key, genes at
	// $.genes, nodes at $.nodes, synapses at $.synapses, five node keys.
	template := `{
	  "name": "Worked", "desc": "a worked example", "isOfficial": false, "nodeAnchors": [],
	  "genes": { "SizeRatio": 1.0, "ColorG": 0.5, "ColorB": 0.30000001192092896 },
	  "nodes": [
	    { "Type": 3, "baseActivation": -0.25, "Index": 48, "Inov": 117, "Desc": "Hidden1" },
	    { "Type": 0, "baseActivation": 0.0, "Index": 0, "Inov": 0, "Desc": "Constant" }
	  ],
	  "synapses": [ { "Inov": 204, "NodeIn": 0, "NodeOut": 48, "Weight": -1.5, "En": true } ],
	  "version": "0.6.3.1"
	}`

	// Case 4 as a table entry; the dedicated test above checks it in detail.
	mutant := strings.Replace(workedExample, "0.30000001192092896", "0.3000001", 1)

	// Case 5: a node description containing "<". Go's encoding/json would write
	// < here, which would split the lineage graph.
	angle := blob(`{"SizeRatio":1.0}`,
		`{"Type":0,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":"a<b"}`, ``)

	// Case 6: a gene value of -0.0.
	negZero := blob(`{"SizeRatio":-0.0}`,
		`{"Type":0,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`, ``)

	// Case 7: a gene value that overflows binary32.
	overflow := blob(`{"SizeRatio":1e39}`,
		`{"Type":0,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`, ``)

	// Case 8: two nodes sharing an index.
	dupIndex := blob(`{"SizeRatio":1.0}`,
		`{"Type":0,"baseActivation":0.0,"Index":5,"Inov":0,"Desc":"a"},`+
			`{"Type":1,"baseActivation":0.0,"Index":5,"Inov":1,"Desc":"b"}`, ``)

	// Case 9: the same blob as a Windows file — CRLF line endings and a UTF-8
	// text layout — must hash to the same digest. The suite cannot run on
	// Windows here, so the platform-dependent inputs are exercised directly:
	// line endings, indentation, and the big-endian bit spelling that removes
	// CPU byte order from the problem.
	windows := strings.ReplaceAll(workedExample, "\n", "\r\n")

	cases := []struct {
		name    string
		payload string
		version string
		want    string // "" means unhashable
		canon   func(t *testing.T, canonical string)
	}{
		{name: "1 the worked example", payload: workedExample, want: workedHash,
			canon: func(t *testing.T, c string) {
				if c != workedCanonical {
					t.Errorf("canonical string = %s", c)
				}
			}},
		{name: "2 nodes and gene keys reordered", payload: reordered, want: workedHash},
		{name: "3 the template dialect", payload: template, want: workedHash,
			canon: func(t *testing.T, c string) {
				if c != workedCanonical {
					t.Errorf("the template canonical string is not byte-identical to the saved one:\n%s", c)
				}
			}},
		{name: "4 the one-ULP mutation", payload: mutant, want: mutantHash},
		{name: "5 a raw < in a node desc", payload: angle, want: "-",
			canon: func(t *testing.T, c string) {
				if !strings.Contains(c, `"desc":"a<b"`) {
					t.Errorf("desc was escaped: %s", c)
				}
				if strings.Contains(c, "\\u003c") {
					t.Errorf("< was escaped as \\u003c: %s", c)
				}
			}},
		{name: "6 a gene value of -0.0", payload: negZero, want: "-",
			canon: func(t *testing.T, c string) {
				if !strings.Contains(c, `"SizeRatio":"f32:00000000"`) {
					t.Errorf("-0.0 was not normalized to +0.0: %s", c)
				}
			}},
		{name: "7 a gene value of 1e39", payload: overflow, want: ""},
		{name: "8 two nodes with Index 5", payload: dupIndex, want: ""},
		{name: "9 the same blob with Windows line endings", payload: windows, want: workedHash},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canon, err := Canonical(tc.payload, tc.version)
			hash, hashErr := GenomeHash(tc.payload, tc.version)
			if tc.want == "" {
				if !errors.Is(err, ErrUnhashableGenome) {
					t.Fatalf("Canonical error = %v, want ErrUnhashableGenome", err)
				}
				if !errors.Is(hashErr, ErrUnhashableGenome) {
					t.Fatalf("GenomeHash error = %v, want ErrUnhashableGenome", hashErr)
				}
				if hash != "" {
					t.Fatalf("an unhashable genome returned %q; §8 forbids a partial hash", hash)
				}
				return
			}
			if err != nil {
				t.Fatalf("Canonical: %v", err)
			}
			if tc.want != "-" && hash != tc.want {
				t.Fatalf("genomeHash = %s, want %s", hash, tc.want)
			}
			if tc.canon != nil {
				tc.canon(t, canon)
			}
		})
	}
}

// TestUnhashableCauses covers the rest of genome-hash.md §8's table.
func TestUnhashableCauses(t *testing.T) {
	cases := map[string]string{
		"neither dialect": `{"version":"0.6.3.1"}`,
		"no node array": `{"body":{},"genes":{"genes":{"a":1.0}},` +
			`"brain":{"Synapses":[]},"version":"0.6.3.1"}`,
		"node without Index": blob(`{"a":1.0}`, `{"Type":0,"Inov":0,"Desc":""}`, ``),
		"unknown type name": blob(`{"a":1.0}`,
			`{"TypeName":"Quantum","baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`, ``),
		"synapse without NodeIn": blob(`{"a":1.0}`,
			`{"Type":0,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`,
			`{"Inov":1,"NodeOut":0,"Weight":1.0,"En":true}`),
		"synapse without Weight": blob(`{"a":1.0}`,
			`{"Type":0,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`,
			`{"Inov":1,"NodeIn":0,"NodeOut":0,"En":true}`),
		"duplicate synapse triple": blob(`{"a":1.0}`,
			`{"Type":0,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`,
			`{"Inov":1,"NodeIn":0,"NodeOut":0,"Weight":1.0,"En":true},`+
				`{"Inov":1,"NodeIn":0,"NodeOut":0,"Weight":2.0,"En":false}`),
		"gene value NaN as a string": blob(`{"a":"NaN"}`,
			`{"Type":0,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`, ``),
		"no version anywhere": `{"body":{},"genes":{"genes":{"a":1.0}},` +
			`"brain":{"Nodes":[{"Type":0,"Index":0}],"Synapses":[]}}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := GenomeHash(payload, ""); !errors.Is(err, ErrUnhashableGenome) {
				t.Fatalf("error = %v, want ErrUnhashableGenome", err)
			}
		})
	}
}

// TestUnknownOrdinalIsProjectedUnchanged covers §4.1: a future game version may
// add a NodeType, and refusing to hash it would be worse than hashing it.
func TestUnknownOrdinalIsProjectedUnchanged(t *testing.T) {
	payload := blob(`{"a":1.0}`, `{"Type":99,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`, ``)
	canon, err := Canonical(payload, "")
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	if !strings.Contains(canon, `"type":99`) {
		t.Fatalf("unknown ordinal was not projected unchanged: %s", canon)
	}
}

// TestDisabledSynapseAndRefWeight covers §4.2: a disabled synapse stays, and a
// gene-reference weight is tagged rather than dropped.
func TestDisabledSynapseAndRefWeight(t *testing.T) {
	payload := blob(`{"a":1.0}`,
		`{"Type":0,"baseActivation":0.0,"Index":0,"Inov":0,"Desc":""}`,
		`{"Inov":7,"NodeIn":0,"NodeOut":0,"Weight":"SizeRatio","En":false}`)
	canon, err := Canonical(payload, "")
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	if !strings.Contains(canon, `"en":false`) {
		t.Fatalf("a disabled synapse was dropped: %s", canon)
	}
	if !strings.Contains(canon, `"weight":"ref:SizeRatio"`) {
		t.Fatalf("a reference weight was not tagged: %s", canon)
	}
}

// TestVersionFallsBackToTheCallersGameVersion covers §3: the caller's
// authoritative gameVersion fills in when the blob carries no $.version.
func TestVersionFallsBackToTheCallersGameVersion(t *testing.T) {
	payload := `{"body":{},"genes":{"genes":{"a":1.0}},` +
		`"brain":{"Nodes":[{"Type":0,"Index":0}],"Synapses":[]}}`
	canon, err := Canonical(payload, "0.6.3.1")
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	if !strings.Contains(canon, `"version":"0.6.3.1"`) {
		t.Fatalf("the authoritative gameVersion was not used: %s", canon)
	}
}

// TestHashHexRejectsAForeignLabel covers §6: an unknown label is an opaque
// foreign identifier, never a bb8-genome/1 value.
func TestHashHexRejectsAForeignLabel(t *testing.T) {
	if HashHex(workedHash) != workedHash[len(HashPrefix):] {
		t.Fatal("HashHex did not return the digest of a valid hash")
	}
	if HashHex("bb8-genome/2:sha256:"+strings.Repeat("a", 64)) != "" {
		t.Fatal("a foreign label must not parse as bb8-genome/1")
	}
	if ValidGenomeHash("bb8-genome/1:sha256:short") {
		t.Fatal("a truncated digest must not validate")
	}
}

// blob builds a minimal saved-dialect blob from a gene object, a node list and
// a synapse list.
func blob(genes, nodes, synapses string) string {
	return `{"body":{"id":-1},"genes":{"gen":1,"genes":` + genes + `},` +
		`"brain":{"Nodes":[` + nodes + `],"Synapses":[` + synapses + `]},` +
		`"version":"0.6.3.1"}`
}
