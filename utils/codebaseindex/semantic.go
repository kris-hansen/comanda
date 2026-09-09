package codebaseindex

import (
	"fmt"
	"regexp"
	"strings"
)

// SemanticGraph is an additive extension to SymbolInfo, independent of language.
// Entity IDs are file-local unless Scope is "project". Relation endpoints use
// an entity ID or $file, $function:Name, $type:Name, $import:Name, $name:Name or $callable:Name.
// $name resolves a unique data symbol locally, then through transitive imports;
// unresolved/ambiguous names remain explicit unresolved_reference nodes.
type SemanticGraph struct {
	Entities  []SemanticEntity
	Relations []SemanticRelation
	// ScopeImports restricts named lookup to lexical dependencies. Nil inherits
	// Imports; an explicit empty slice searches only local declarations.
	ScopeImports []string
}

type SemanticEntity struct {
	ID            string
	Kind          string
	Name          string
	Summary       string
	Scope         string `json:",omitempty"` // empty/file or project (shared within language)
	Referenceable bool   `json:",omitempty"` // participates in $name resolution
}

type SemanticRelation struct {
	Source     string
	Target     string
	Kind       string
	Confidence string // extracted or inferred; required, never implicitly upgraded
	Evidence   string // required source location and explanation supplied by parser
}

var semanticKind = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidateSemanticGraph rejects malformed graphs at the parser boundary.
func ValidateSemanticGraph(symbols *SymbolInfo) error {
	if symbols == nil || symbols.SemanticGraph == nil {
		return nil
	}
	refs := map[string]bool{"$file": true}
	for _, t := range symbols.Types {
		refs["$type:"+t.Name] = true
	}
	for _, f := range symbols.Functions {
		name := f.Name
		if f.IsMethod && f.Receiver != "" {
			name = f.Receiver + "." + name
		}
		refs["$function:"+name] = true
	}
	for _, imp := range symbols.Imports {
		refs["$import:"+imp] = true
	}
	for _, imp := range symbols.SemanticGraph.ScopeImports {
		if !refs["$import:"+imp] {
			return fmt.Errorf("scope import %q is not declared in Imports", imp)
		}
	}
	for _, e := range symbols.SemanticGraph.Entities {
		if strings.TrimSpace(e.ID) == "" || strings.HasPrefix(e.ID, "$") || strings.Contains(e.ID, "|") || refs[e.ID] {
			return fmt.Errorf("invalid or duplicate entity ID %q", e.ID)
		}
		if !semanticKind.MatchString(e.Kind) || strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("entity %q needs a valid kind and name", e.ID)
		}
		if e.Scope != "" && e.Scope != "file" && e.Scope != "project" {
			return fmt.Errorf("entity %q has invalid scope %q", e.ID, e.Scope)
		}
		refs[e.ID] = true
	}
	for _, r := range symbols.SemanticGraph.Relations {
		if !semanticKind.MatchString(r.Kind) || strings.TrimSpace(r.Evidence) == "" {
			return fmt.Errorf("relation %q needs a valid kind and evidence", r.Kind)
		}
		if r.Confidence != "extracted" && r.Confidence != "inferred" {
			return fmt.Errorf("invalid relation confidence %q", r.Confidence)
		}
		for _, ref := range []string{r.Source, r.Target} {
			if refs[ref] {
				continue
			}
			if (strings.HasPrefix(ref, "$name:") || strings.HasPrefix(ref, "$callable:")) && strings.TrimSpace(strings.SplitN(ref, ":", 2)[1]) != "" && !strings.Contains(ref, "|") {
				continue
			}
			return fmt.Errorf("unknown semantic endpoint %q", ref)
		}
	}
	return nil
}
