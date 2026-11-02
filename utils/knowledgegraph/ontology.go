package knowledgegraph

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

const DocumentOntologyVersion = 1

var documentRelations = map[string]struct {
	targets map[string]bool
}{
	EdgeSupersedes: {targets: kinds(NodeDocument)},
	EdgeDependsOn:  {targets: kinds(NodeDocument)},
	EdgeRelatedTo:  {targets: kinds(NodeDocument)},
	EdgeDescribes: {targets: kinds(
		NodeDocument, NodeComponent, NodePackage, NodeFile, NodeType, NodeFunction,
		"schema", "table", "column", "concept",
	)},
	EdgeGuides: {targets: kinds(NodeComponent, NodePackage, NodeFile)},
}

func kinds(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

// ValidateDocumentRelation checks the versioned built-in document ontology.
func ValidateDocumentRelation(kind, targetKind string) error {
	relation, ok := documentRelations[kind]
	if !ok {
		return fmt.Errorf("unsupported document relation %q", kind)
	}
	if !relation.targets[targetKind] {
		return fmt.Errorf("relation %s cannot target node kind %s", kind, targetKind)
	}
	return nil
}

func setNodeProperties(node *semanticmemory.GraphNode, properties map[string]any) {
	if len(properties) == 0 {
		return
	}
	encoded, err := json.Marshal(properties)
	if err == nil {
		node.Properties = string(encoded)
	}
}

// AddEvidence attaches a stable, independently retained citation to an edge.
func (g *Graph) AddEvidence(edgeID string, evidence semanticmemory.GraphEvidence) {
	if g.Edges[edgeID] == nil {
		return
	}
	evidence.Namespace = g.Namespace
	evidence.EdgeID = edgeID
	if evidence.Extractor == "" {
		evidence.Extractor = "document_ast"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%d|%s|%s", edgeID, evidence.SourcePath,
		evidence.StartLine, evidence.EndLine, evidence.Quote, evidence.Extractor)))
	evidence.ID = fmt.Sprintf("evidence:%x", sum[:12])
	for _, existing := range g.Evidence {
		if existing.ID == evidence.ID {
			return
		}
	}
	g.Evidence = append(g.Evidence, evidence)
}

func (g *Graph) addDiagnostic(kind, path string, line int, format string, args ...any) {
	g.Diagnostics = append(g.Diagnostics, GraphDiagnostic{
		Kind: kind, Path: path, Line: line, Message: fmt.Sprintf(format, args...),
	})
}
