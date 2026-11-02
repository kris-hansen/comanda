package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kris-hansen/comanda/utils/knowledgegraph"
)

func TestWriteGraphBuildReport(t *testing.T) {
	graph := knowledgegraph.NewGraph("docs")
	graph.Diagnostics = append(graph.Diagnostics, knowledgegraph.GraphDiagnostic{
		Kind: "unresolved_link", Path: "guide.md", Line: 4, Message: "missing target",
	})
	path := filepath.Join(t.TempDir(), "memory.db")
	if err := writeGraphBuildReport(path, graph); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path + ".graph-build.json")
	if err != nil {
		t.Fatal(err)
	}
	var report graphBuildReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Namespace != "docs" || report.OntologyVersion != knowledgegraph.DocumentOntologyVersion || len(report.Diagnostics) != 1 {
		t.Fatalf("unexpected build report: %#v", report)
	}
}
