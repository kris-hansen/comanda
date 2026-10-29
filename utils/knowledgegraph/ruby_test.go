package knowledgegraph

import (
	"testing"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
)

func TestRubyGraphKeepsClassAndInstanceMethodsDistinct(t *testing.T) {
	adapter := &codebaseindex.RubyAdapter{}
	symbols, err := adapter.ExtractSymbols("lib/cart.rb", []byte("require 'json'\nmodule Shop\nclass Cart\ndef build; end\ndef self.build; end\nend\nend\n"))
	if err != nil {
		t.Fatal(err)
	}
	scan := &codebaseindex.ScanResult{Candidates: []*codebaseindex.FileEntry{{Path: "lib/cart.rb", Language: "ruby", Symbols: symbols}}}
	g := Build(scan, "ruby")
	typeID := NodeID("ruby", "type:Shop::Cart@lib/cart.rb")
	for _, name := range []string{"Shop::Cart.build", "Shop::Cart.self.build"} {
		fnID := NodeID("ruby", "func:"+name+"@lib/cart.rb")
		if g.Nodes[fnID] == nil || g.Edges[EdgeID("ruby", typeID, fnID, EdgeDefines)] == nil {
			t.Errorf("missing method or receiver edge: %s", name)
		}
	}
	if g.Edges[EdgeID("ruby", NodeID("ruby", "file:lib/cart.rb"), NodeID("ruby", "pkg:json"), EdgeImports)] == nil {
		t.Fatal("missing Ruby dependency edge")
	}
}
