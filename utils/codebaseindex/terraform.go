package codebaseindex

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// extractTerraformSymbols reads Terraform's native HCL syntax rather than
// guessing from text. Its graph symbols use the references Terraform users
// write: aws_s3_bucket.logs, data.aws_ami.current, module.network, var.region,
// and local.tags.
func extractTerraformSymbols(path string, content []byte) (*SymbolInfo, error) {
	file, diagnostics := hclsyntax.ParseConfig(content, path, hcl.Pos{Line: 1, Column: 1})
	if diagnostics.HasErrors() {
		return nil, fmt.Errorf("parse Terraform HCL: %s", diagnostics.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("parse Terraform HCL: unexpected body type")
	}

	info := &SymbolInfo{Package: "terraform"}
	references := make(map[string]struct{})
	for _, block := range body.Blocks {
		name, kind, variable := terraformBlockSymbol(block)
		if name != "" {
			info.Types = append(info.Types, TypeInfo{Name: name, Kind: kind, Fields: sortedAttributeNames(block.Body.Attributes)})
			if variable {
				info.Variables = append(info.Variables, name)
			}
		}
		collectTerraformReferences(references, block.Body.Attributes)
	}
	// locals are attributes in one or more locals blocks, not labelled blocks.
	for _, block := range body.Blocks {
		if block.Type != "locals" {
			continue
		}
		for _, name := range sortedAttributeNames(block.Body.Attributes) {
			local := "local." + name
			info.Types = append(info.Types, TypeInfo{Name: local, Kind: "local", Fields: []string{name}})
			info.Variables = append(info.Variables, local)
		}
	}
	collectTerraformReferences(references, body.Attributes)
	for reference := range references {
		info.References = append(info.References, reference)
	}
	sort.Strings(info.References)
	sort.Strings(info.Variables)
	sort.Slice(info.Types, func(i, j int) bool { return info.Types[i].Name < info.Types[j].Name })
	return info, nil
}

func terraformBlockSymbol(block *hclsyntax.Block) (name, kind string, variable bool) {
	label := func(index int) string {
		if len(block.Labels) <= index {
			return ""
		}
		return block.Labels[index]
	}
	switch block.Type {
	case "resource":
		if label(0) != "" && label(1) != "" {
			return label(0) + "." + label(1), "resource", false
		}
	case "data":
		if label(0) != "" && label(1) != "" {
			return "data." + label(0) + "." + label(1), "data source", false
		}
	case "module":
		if label(0) != "" {
			return "module." + label(0), "module", false
		}
	case "variable":
		if label(0) != "" {
			return "var." + label(0), "variable", true
		}
	case "output":
		if label(0) != "" {
			return "output." + label(0), "output", false
		}
	case "provider":
		if label(0) != "" {
			return "provider." + label(0), "provider", false
		}
	}
	return "", "", false
}

func sortedAttributeNames(attributes hclsyntax.Attributes) []string {
	names := make([]string, 0, len(attributes))
	for name := range attributes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func collectTerraformReferences(references map[string]struct{}, attributes hclsyntax.Attributes) {
	for _, attribute := range attributes {
		for _, traversal := range attribute.Expr.Variables() {
			if reference := terraformTraversalReference(traversal); reference != "" {
				references[reference] = struct{}{}
			}
		}
	}
}

func terraformTraversalReference(traversal hcl.Traversal) string {
	if len(traversal) == 0 {
		return ""
	}
	parts := make([]string, 0, len(traversal))
	for _, step := range traversal {
		switch step := step.(type) {
		case hcl.TraverseRoot:
			parts = append(parts, step.Name)
		case hcl.TraverseAttr:
			parts = append(parts, step.Name)
		}
	}
	if len(parts) < 2 || terraformBuiltinReference(parts[0]) {
		return ""
	}
	want := 2 // resource_type.resource_name, var.name, module.name
	if parts[0] == "data" {
		want = 3
	}
	if len(parts) < want {
		return ""
	}
	return strings.Join(parts[:want], ".")
}

func terraformBuiltinReference(root string) bool {
	switch root {
	case "path", "terraform", "each", "count", "self":
		return true
	default:
		return false
	}
}
