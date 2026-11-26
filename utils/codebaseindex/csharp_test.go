package codebaseindex

import (
	"reflect"
	"strings"
	"testing"
)

func TestCSharpDeclarationsAndScopes(t *testing.T) {
	source := `using System;
using System.Collections.Generic;
using External = Example.External.Dependency;
namespace Shop.Services;

/// <summary>Cart service.</summary>
public partial class Cart : CartBase
{
    public const string Currency = "USD";
    private readonly int retryCount = 3;
    public string Name { get; set; }
    public int Count => items.Count;

    public Cart(string name) { Name = name; }

    // Calculates a total.
    public async Task<decimal> TotalAsync<T>(T item) where T : IItem
    {
        if (item == null) { return 0; }
        return item.Price;
    }

    private void Reset() { items.Clear(); }

    public class Nested
    {
        public void Run() { }
    }
}

public record struct Point(int X, int Y);
public interface IService { void Run(); }
public enum Status { Active, Disabled = 2 }
public delegate void Changed(object sender);
`
	info := extractCSharpSymbols("Services/Cart.cs", []byte(source))
	if info.Package != "Shop.Services" {
		t.Fatalf("package: %q", info.Package)
	}
	if !reflect.DeepEqual(info.Imports, []string{"System", "System.Collections.Generic", "Example.External.Dependency"}) {
		t.Fatalf("imports: %v", info.Imports)
	}
	var types []string
	for _, typ := range info.Types {
		types = append(types, typ.Name)
	}
	wantTypes := []string{"Shop.Services.Cart", "Shop.Services.Cart.Nested", "Shop.Services.Point", "Shop.Services.IService", "Shop.Services.Status", "Shop.Services.Changed"}
	if !reflect.DeepEqual(types, wantTypes) {
		t.Fatalf("types: %v, want %v", types, wantTypes)
	}
	if info.Types[0].Comments != "<summary>Cart service.</summary>" {
		t.Fatalf("type comments: %q", info.Types[0].Comments)
	}
	if !reflect.DeepEqual(info.Types[0].Fields, []string{"Currency", "retryCount", "Name", "Count"}) {
		t.Fatalf("fields: %v", info.Types[0].Fields)
	}
	wantConstants := []string{"Shop.Services.Cart.Currency", "Shop.Services.Status.Active", "Shop.Services.Status.Disabled"}
	if !reflect.DeepEqual(info.Constants, wantConstants) {
		t.Fatalf("constants: %v", info.Constants)
	}
	wantFunctions := []struct {
		name, receiver string
		exported       bool
	}{
		{"Cart", "Shop.Services.Cart", true},
		{"TotalAsync", "Shop.Services.Cart", true},
		{"Reset", "Shop.Services.Cart", false},
		{"Run", "Shop.Services.Cart.Nested", true},
		{"Run", "Shop.Services.IService", true},
	}
	if len(info.Functions) != len(wantFunctions) {
		t.Fatalf("functions: %+v", info.Functions)
	}
	for i, fn := range info.Functions {
		want := wantFunctions[i]
		if fn.Name != want.name || fn.Receiver != want.receiver || fn.IsExported != want.exported || !fn.IsMethod {
			t.Errorf("function %d: %+v, want %+v", i, fn, want)
		}
	}
	if info.Functions[1].Signature != "public async Task<decimal> TotalAsync<T>(T item)" || info.Functions[1].Comments != "Calculates a total." {
		t.Fatalf("signature/comments: %+v", info.Functions[1])
	}
}

func TestCSharpBlockNamespacesAndNonCode(t *testing.T) {
	source := `// class CommentFake { public void Fake() { } }
/* class BlockFake { } */
namespace Outer
{
    namespace Inner
    {
        public class Real
        {
            public void Run()
            {
                var regular = "class StringFake { public void Fake() { } }";
                var verbatim = @"class VerbatimFake { }";
                var raw = """class RawFake { public void Fake() { } }""";
                var interpolated = $"{regular.Length} class InterpolatedFake {{ }}";
                var character = '}';
            }
        }
    }
}
`
	info := extractCSharpSymbols("Real.cs", []byte(source))
	if info.Package != "Outer.Inner" || len(info.Types) != 1 || info.Types[0].Name != "Outer.Inner.Real" {
		t.Fatalf("unexpected types: %+v", info)
	}
	if len(info.Functions) != 1 || info.Functions[0].Name != "Run" || info.Functions[0].Receiver != "Outer.Inner.Real" {
		t.Fatalf("non-code leaked into functions: %+v", info.Functions)
	}
}

func TestCSharpAdapterScan(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{
		"Shop.sln":                  "Microsoft Visual Studio Solution File\n",
		"Shop.csproj":               "<Project Sdk=\"Microsoft.NET.Sdk.Web\"></Project>\n",
		"Program.cs":                "using Microsoft.AspNetCore.Builder;\npublic class Program { public static void Main(string[] args) { } }\n",
		"Services/User.CS":          "namespace Shop; public class User { public void Save() { } }\n",
		"Services/User.g.cs":        "namespace Shop; public class Generated { }\n",
		"obj/Generated.cs":          "namespace Shop; public class Excluded { }\n",
		"bin/Compiled.cs":           "namespace Shop; public class Excluded { }\n",
		".vs/Workspace.cs":          "namespace Shop; public class Excluded { }\n",
		"TestResults/Results.cs":    "namespace Shop; public class Excluded { }\n",
		"packages/Dependency.cs":    "namespace Shop; public class Excluded { }\n",
		"Services/Generated.g.i.cs": "namespace Shop; public class Excluded { }\n",
	} {
		writeStructureFixture(t, root, path, content)
	}
	cfg := DefaultConfig()
	cfg.Root = root
	cfg.OutputFormat = FormatFull
	m, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	scan, languages, err := m.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(languages, []string{"csharp"}) || len(scan.GraphFiles) != 4 {
		t.Fatalf("languages %v, files %+v", languages, scan.GraphFiles)
	}
	for _, file := range scan.GraphFiles {
		switch file.Path {
		case "Shop.sln", "Shop.csproj":
			if file.Language != "" || !file.IsConfig {
				t.Errorf("project file incorrectly classified: %+v", file)
			}
		default:
			if file.Language != "csharp" || file.Symbols == nil {
				t.Errorf("source file not parsed: %+v", file)
			}
		}
	}
	if len(scan.Components) != 1 || scan.Components[0].Language != "csharp" || scan.Components[0].Kind != "backend" {
		t.Fatalf("components: %+v", scan.Components)
	}
	result, err := m.Generate()
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Shop.User", "Save", "aspnetcore"} {
		if !strings.Contains(result.Content, text) {
			t.Errorf("index omitted %q", text)
		}
	}
}

func TestCSharpDetectionWithoutSource(t *testing.T) {
	for _, test := range []struct {
		name, path string
	}{
		{"project", "Shop.csproj"},
		{"solution", "Shop.sln"},
		{"XML solution", "Shop.slnx"},
		{"nested project", "service/Shop.csproj"},
		{"global JSON", "global.json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeStructureFixture(t, root, test.path, "<Project/>\n")
			adapters := NewRegistry().Detect(root)
			if len(adapters) != 1 || adapters[0].Name() != "csharp" {
				t.Fatalf("adapters: %v", adapters)
			}
		})
	}
}

func TestCSharpFrameworksAndRisks(t *testing.T) {
	source := `using Microsoft.AspNetCore.Authentication;
using Microsoft.EntityFrameworkCore;
using System.Security.Cryptography;
using System.Net.Http;
using System.Diagnostics;
using Xunit;
namespace Shop;
public class Service { }
`
	info := extractCSharpSymbols("Service.cs", []byte(source))
	if !reflect.DeepEqual(info.Frameworks, []string{"aspnetcore", "entityframework", "xunit"}) {
		t.Fatalf("frameworks: %v", info.Frameworks)
	}
	if !reflect.DeepEqual(info.RiskTags, []string{"auth", "database", "crypto", "network", "subprocess"}) {
		t.Fatalf("risk tags: %v", info.RiskTags)
	}
}

func FuzzCSharpExtraction(f *testing.F) {
	for _, source := range []string{"", "namespace", "class", "public class C {", "public class C { void M(", "record R(", "using X =", "@\"", "\"\"\""} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		info := extractCSharpSymbols("input.cs", []byte(source))
		if info == nil {
			t.Fatal("nil symbols")
		}
	})
}
