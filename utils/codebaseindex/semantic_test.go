package codebaseindex

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSemanticGraphProtocolValidation(t *testing.T) {
	input := `{"Package":"demo","Functions":[{"Name":"Run"}],"Imports":["COPY"],"SemanticGraph":{"Entities":[{"ID":"field:x","Kind":"data_item","Name":"X","Referenceable":true}],"Relations":[{"Source":"$function:Run","Target":"field:x","Kind":"writes","Confidence":"extracted","Evidence":"demo:3: MOVE 1 TO X"}]}}`
	var s SymbolInfo
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSemanticGraph(&s); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"$name:IMPORTED", "$callable:REMOTE", "$import:COPY", "$file"} {
		s.SemanticGraph.Relations[0].Target = ref
		if err := ValidateSemanticGraph(&s); err != nil {
			t.Errorf("%s: %v", ref, err)
		}
	}
	for _, ref := range []string{"missing", "$function:Missing", "$name:", "$import:UNDECLARED", "$name:x|other"} {
		s.SemanticGraph.Relations[0].Target = ref
		if ValidateSemanticGraph(&s) == nil {
			t.Errorf("accepted missing/invalid endpoint %q", ref)
		}
	}
	s.SemanticGraph.Relations[0].Target = "field:x"
	s.SemanticGraph.Relations[0].Confidence = "certain"
	if ValidateSemanticGraph(&s) == nil {
		t.Fatal("accepted invalid confidence")
	}
	s.SemanticGraph.Relations[0].Confidence = "extracted"
	s.SemanticGraph.Relations[0].Evidence = ""
	if ValidateSemanticGraph(&s) == nil {
		t.Fatal("accepted missing evidence")
	}
	s.SemanticGraph.Relations[0].Evidence = "demo:3"
	s.SemanticGraph.Entities = append(s.SemanticGraph.Entities, s.SemanticGraph.Entities[0])
	if ValidateSemanticGraph(&s) == nil {
		t.Fatal("accepted duplicate ID")
	}
	if err := ValidateSemanticGraph(&SymbolInfo{}); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticIndexOverview(t *testing.T) {
	s := &SymbolInfo{SemanticGraph: &SemanticGraph{
		Entities:  []SemanticEntity{{ID: "interest", Kind: "concept", Name: "Interest", Scope: "project"}},
		Relations: []SemanticRelation{{Source: "$file", Target: "interest", Kind: "calculates", Confidence: "inferred", Evidence: "main:12: COMPUTE RESULT = RATE * BALANCE"}},
	}}
	var out strings.Builder
	m := &Manager{}
	m.writeSemanticOverview(&out, &ScanResult{GraphFiles: []*FileEntry{{Path: "main.cbl", Symbols: s}}})
	for _, want := range []string{"Parser Semantic Model", "Interest", "calculates", "inferred", "main:12"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %s in %s", want, out.String())
		}
	}
}
