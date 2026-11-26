package knowledgegraph

import (
	"testing"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
)

func TestCSharpGraphLinksQualifiedMethodsAndDependencies(t *testing.T) {
	adapter := &codebaseindex.CSharpAdapter{}
	symbols, err := adapter.ExtractSymbols("Services/Cart.cs", []byte(`using Microsoft.AspNetCore.Builder;
namespace Shop.Services;
public class Cart { public void Save() { } }
`))
	if err != nil {
		t.Fatal(err)
	}
	scan := &codebaseindex.ScanResult{Candidates: []*codebaseindex.FileEntry{{Path: "Services/Cart.cs", Language: "csharp", Symbols: symbols}}}
	graph := Build(scan, "csharp")
	typeID := NodeID("csharp", "type:Shop.Services.Cart@Services/Cart.cs")
	functionID := NodeID("csharp", "func:Shop.Services.Cart.Save@Services/Cart.cs")
	if graph.Nodes[typeID] == nil || graph.Nodes[functionID] == nil {
		t.Fatalf("missing C# nodes: types=%v functions=%v", graph.Nodes[typeID], graph.Nodes[functionID])
	}
	if graph.Edges[EdgeID("csharp", typeID, functionID, EdgeDefines)] == nil {
		t.Fatal("missing C# method ownership edge")
	}
	if graph.Edges[EdgeID("csharp", NodeID("csharp", "file:Services/Cart.cs"), NodeID("csharp", "pkg:Microsoft.AspNetCore.Builder"), EdgeImports)] == nil {
		t.Fatal("missing C# dependency edge")
	}
}
