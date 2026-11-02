package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kris-hansen/comanda/utils/knowledgegraph"
)

type graphBuildReport struct {
	Namespace       string                           `json:"namespace"`
	OntologyVersion int                              `json:"ontology_version"`
	Database        string                           `json:"database"`
	GeneratedAt     time.Time                        `json:"generated_at"`
	Nodes           int                              `json:"nodes"`
	Edges           int                              `json:"edges"`
	Evidence        int                              `json:"evidence"`
	Diagnostics     []knowledgegraph.GraphDiagnostic `json:"diagnostics"`
}

func writeGraphBuildReport(dbPath string, graph *knowledgegraph.Graph) error {
	diagnostics := graph.Diagnostics
	if diagnostics == nil {
		diagnostics = []knowledgegraph.GraphDiagnostic{}
	}
	report := graphBuildReport{
		Namespace: graph.Namespace, OntologyVersion: graph.OntologyVersion, Database: dbPath,
		GeneratedAt: time.Now().UTC(), Nodes: len(graph.Nodes), Edges: len(graph.Edges),
		Evidence: len(graph.Evidence), Diagnostics: diagnostics,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	reportPath := dbPath + ".graph-build.json"
	tmp, err := os.CreateTemp(filepath.Dir(dbPath), ".graph-build-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, reportPath)
}
