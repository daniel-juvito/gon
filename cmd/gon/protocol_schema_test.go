package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Schema-level JSON wire tests for Diagnostic Protocol v1.
// These do not run the checker or CLI; they assert the wire contract only.

func TestProtocol_RelatedInformation_SchemaRoundTrip(t *testing.T) {
	// Construct → Marshal → inspect wire keys → Unmarshal → nested asserts.
	// A typo in the json struct tag (e.g. related_information) must fail here.
	orig := protocolDiagnostic{
		Code:     "GN001",
		Severity: "error",
		Message:  "cannot assign nil to non-nil type !*int",
		Source:   "gon-check",
		File:     "/abs/path/to/main.gon",
		Range: protocolRange{
			Start: protocolPos{Line: 2, Column: 10},
			End:   protocolPos{Line: 2, Column: 13},
		},
		RelatedInformation: []protocolRelated{
			{
				Message: "non-nil contract declared here",
				Location: protocolLoc{
					File: "/abs/path/to/config.gna",
					Range: protocolRange{
						Start: protocolPos{Line: 4, Column: 8},
						End:   protocolPos{Line: 4, Column: 16},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Wire-key contract: camelCase relatedInformation, not snake_case.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("unmarshal top-level map: %v", err)
	}
	if _, ok := top["relatedInformation"]; !ok {
		t.Fatalf(`wire key "relatedInformation" missing; raw=%s`, raw)
	}
	if _, ok := top["related_information"]; ok {
		t.Fatalf(`unexpected snake_case wire key "related_information"; raw=%s`, raw)
	}
	// Nested location object must use "file" and "range", not alternate names.
	var riArr []map[string]json.RawMessage
	if err := json.Unmarshal(top["relatedInformation"], &riArr); err != nil {
		t.Fatalf("unmarshal relatedInformation array: %v", err)
	}
	if len(riArr) != 1 {
		t.Fatalf("relatedInformation length: want 1, got %d", len(riArr))
	}
	if _, ok := riArr[0]["message"]; !ok {
		t.Fatalf(`relatedInformation[0]: missing wire key "message"`)
	}
	if _, ok := riArr[0]["location"]; !ok {
		t.Fatalf(`relatedInformation[0]: missing wire key "location"`)
	}
	var loc map[string]json.RawMessage
	if err := json.Unmarshal(riArr[0]["location"], &loc); err != nil {
		t.Fatalf("unmarshal location: %v", err)
	}
	if _, ok := loc["file"]; !ok {
		t.Fatalf(`location: missing wire key "file"`)
	}
	if _, ok := loc["range"]; !ok {
		t.Fatalf(`location: missing wire key "range"`)
	}
	var rng map[string]json.RawMessage
	if err := json.Unmarshal(loc["range"], &rng); err != nil {
		t.Fatalf("unmarshal range: %v", err)
	}
	if _, ok := rng["start"]; !ok {
		t.Fatalf(`range: missing wire key "start"`)
	}
	if _, ok := rng["end"]; !ok {
		t.Fatalf(`range: missing wire key "end"`)
	}

	// Guard against accidental snake_case anywhere in the blob for this field path.
	if strings.Contains(string(raw), `"related_information"`) {
		t.Fatalf("raw JSON contains related_information snake_case")
	}

	// Struct round-trip: nested values intact.
	var got protocolDiagnostic
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal protocolDiagnostic: %v", err)
	}
	if len(got.RelatedInformation) != 1 {
		t.Fatalf("relatedInformation: want 1 entry, got %d", len(got.RelatedInformation))
	}
	ri := got.RelatedInformation[0]
	if ri.Message != "non-nil contract declared here" {
		t.Errorf("relatedInformation[0].message: got %q", ri.Message)
	}
	if ri.Location.File != "/abs/path/to/config.gna" {
		t.Errorf("relatedInformation[0].location.file: got %q", ri.Location.File)
	}
	if ri.Location.Range.Start.Line != 4 || ri.Location.Range.Start.Column != 8 {
		t.Errorf("relatedInformation[0].location.range.start: got %+v", ri.Location.Range.Start)
	}
	if ri.Location.Range.End.Line != 4 || ri.Location.Range.End.Column != 16 {
		t.Errorf("relatedInformation[0].location.range.end: got %+v", ri.Location.Range.End)
	}
	if got.Code != orig.Code || got.Severity != orig.Severity || got.Source != orig.Source {
		t.Errorf("required fields drifted after round-trip: %+v", got)
	}
}
