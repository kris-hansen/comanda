package codebaseindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewManager(t *testing.T) {
	config := DefaultConfig()
	config.Root = "."

	manager, err := NewManager(config, false)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if manager.config.RepoFileSlug == "" {
		t.Error("RepoFileSlug should not be empty")
	}

	if manager.config.RepoVarSlug == "" {
		t.Error("RepoVarSlug should not be empty")
	}
}

func TestDeriveRepoSlugs(t *testing.T) {
	tests := []struct {
		input        string
		expectedFile string
		expectedVar  string
	}{
		{"my-project", "my_project", "MY_PROJECT"},
		{"MyProject", "myproject", "MYPROJECT"},
		{"my_project", "my_project", "MY_PROJECT"},
		{"my--project", "my_project", "MY_PROJECT"},
	}

	for _, tt := range tests {
		fileSlug, varSlug := deriveRepoSlugs(tt.input)
		if fileSlug != tt.expectedFile {
			t.Errorf("deriveRepoSlugs(%q) fileSlug = %q, want %q", tt.input, fileSlug, tt.expectedFile)
		}
		if varSlug != tt.expectedVar {
			t.Errorf("deriveRepoSlugs(%q) varSlug = %q, want %q", tt.input, varSlug, tt.expectedVar)
		}
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	// Should have default adapters
	adapters := registry.All()
	if len(adapters) == 0 {
		t.Error("Registry should have default adapters")
	}

	// Should be able to get Go adapter
	goAdapter, ok := registry.Get("go")
	if !ok {
		t.Error("Registry should have Go adapter")
	}
	if goAdapter.Name() != "go" {
		t.Errorf("Go adapter name should be 'go', got %q", goAdapter.Name())
	}
}

func TestGoAdapterDetection(t *testing.T) {
	// Create temp directory with go.mod
	tmpDir := t.TempDir()
	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	detected := registry.Detect(tmpDir)

	foundGo := false
	for _, a := range detected {
		if a.Name() == "go" {
			foundGo = true
			break
		}
	}

	if !foundGo {
		t.Error("Go adapter should be detected for directory with go.mod")
	}
}

func TestMonorepoDetection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create monorepo structure: backend/ with go.mod, webapp/ with package.json
	backendDir := filepath.Join(tmpDir, "backend")
	webappDir := filepath.Join(tmpDir, "webapp")
	if err := os.Mkdir(backendDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(webappDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(backendDir, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webappDir, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	detected := registry.Detect(tmpDir)

	// Should detect both Go and TypeScript
	names := make(map[string]bool)
	for _, a := range detected {
		names[a.Name()] = true
	}

	if !names["go"] {
		t.Error("Should detect Go adapter in backend/ subdirectory")
	}
	if !names["typescript"] {
		t.Error("Should detect TypeScript adapter in webapp/ subdirectory")
	}
}

func TestExtractGoSymbols(t *testing.T) {
	content := []byte(`package main

import (
	"fmt"
	"net/http"
)

type Config struct {
	Name string
}

func main() {
	fmt.Println("hello")
}

func (c *Config) String() string {
	return c.Name
}
`)

	info, err := extractGoSymbols("test.go", content)
	if err != nil {
		t.Fatalf("extractGoSymbols failed: %v", err)
	}

	if info.Package != "main" {
		t.Errorf("Package should be 'main', got %q", info.Package)
	}

	if len(info.Imports) != 2 {
		t.Errorf("Should have 2 imports, got %d", len(info.Imports))
	}

	if len(info.Types) != 1 {
		t.Errorf("Should have 1 type, got %d", len(info.Types))
	}

	if info.Types[0].Name != "Config" {
		t.Errorf("Type name should be 'Config', got %q", info.Types[0].Name)
	}

	if len(info.Functions) != 2 {
		t.Errorf("Should have 2 functions, got %d", len(info.Functions))
	}
}

func TestExtractPythonSymbols(t *testing.T) {
	content := []byte(`import os
from flask import Flask

class MyApp:
    def __init__(self):
        pass

def main():
    app = MyApp()
`)

	info, err := extractPythonSymbols("test.py", content)
	if err != nil {
		t.Fatalf("extractPythonSymbols failed: %v", err)
	}

	if len(info.Imports) == 0 {
		t.Error("Should have imports")
	}

	if len(info.Types) != 1 {
		t.Errorf("Should have 1 class, got %d", len(info.Types))
	}

	if info.Types[0].Name != "MyApp" {
		t.Errorf("Class name should be 'MyApp', got %q", info.Types[0].Name)
	}
}

func TestExtractTypeScriptSymbols(t *testing.T) {
	content := []byte(`import { Component } from 'react';
import express from 'express';

export interface User {
    name: string;
}

export class App extends Component {
}

export function main() {
}
`)

	info, err := extractTypeScriptSymbols("test.ts", content)
	if err != nil {
		t.Fatalf("extractTypeScriptSymbols failed: %v", err)
	}

	if len(info.Imports) == 0 {
		t.Error("Should have imports")
	}

	// Check for interface
	foundInterface := false
	for _, ti := range info.Types {
		if ti.Name == "User" && ti.Kind == "interface" {
			foundInterface = true
			break
		}
	}
	if !foundInterface {
		t.Error("Should have User interface")
	}

	// Check for class
	foundClass := false
	for _, ti := range info.Types {
		if ti.Name == "App" && ti.Kind == "class" {
			foundClass = true
			break
		}
	}
	if !foundClass {
		t.Error("Should have App class")
	}
}

func TestJavaAdapterDetection(t *testing.T) {
	cases := []struct {
		name       string
		filename   string
		fileBody   string
		subdir     string
		shouldFind bool
	}{
		{name: "pom.xml at root", filename: "pom.xml", fileBody: "<project/>", shouldFind: true},
		{name: "build.gradle at root", filename: "build.gradle", fileBody: "apply plugin: 'java'", shouldFind: true},
		{name: "build.gradle.kts at root", filename: "build.gradle.kts", fileBody: "plugins { java }", shouldFind: true},
		{name: "pom.xml in subdir", filename: "pom.xml", fileBody: "<project/>", subdir: "service-a", shouldFind: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dir := tmpDir
			if tc.subdir != "" {
				dir = filepath.Join(tmpDir, tc.subdir)
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, tc.filename), []byte(tc.fileBody), 0644); err != nil {
				t.Fatal(err)
			}

			registry := NewRegistry()
			detected := registry.Detect(tmpDir)

			found := false
			for _, a := range detected {
				if a.Name() == "java" {
					found = true
					break
				}
			}
			if found != tc.shouldFind {
				t.Errorf("Java detection for %s: got %v, want %v", tc.filename, found, tc.shouldFind)
			}
		})
	}
}

func TestExtractJavaSymbols(t *testing.T) {
	content := []byte(`package com.example.demo;

import java.util.List;
import java.util.concurrent.ConcurrentHashMap;
import org.springframework.boot.SpringApplication;
import static java.util.Arrays.asList;

/**
 * App entrypoint.
 */
public class DemoApplication {
    public static final String VERSION = "1.0";
    private static final int MAX_RETRIES = 3;

    public static void main(String[] args) {
        SpringApplication.run(DemoApplication.class, args);
    }

    private String greet(String name) {
        return "Hello, " + name;
    }
}

interface Greeter {
    String greet(String name);
}

enum Status { OK, ERROR }
`)

	info, err := extractJavaSymbols("DemoApplication.java", content)
	if err != nil {
		t.Fatalf("extractJavaSymbols failed: %v", err)
	}

	if info.Package != "com.example.demo" {
		t.Errorf("Package should be 'com.example.demo', got %q", info.Package)
	}

	if len(info.Imports) != 4 {
		t.Errorf("Should have 4 imports, got %d (%v)", len(info.Imports), info.Imports)
	}

	wantTypes := map[string]string{
		"DemoApplication": "class",
		"Greeter":         "interface",
		"Status":          "enum",
	}
	for name, kind := range wantTypes {
		found := false
		for _, ti := range info.Types {
			if ti.Name == name && ti.Kind == kind {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Should have %s %s in types %+v", kind, name, info.Types)
		}
	}

	wantMethods := map[string]bool{"main": true, "greet": true}
	for _, fn := range info.Functions {
		delete(wantMethods, fn.Name)
	}
	if len(wantMethods) != 0 {
		t.Errorf("Missing expected methods: %v (got %+v)", wantMethods, info.Functions)
	}

	foundConst := false
	for _, c := range info.Constants {
		if c == "VERSION" || c == "MAX_RETRIES" {
			foundConst = true
		}
	}
	if !foundConst {
		t.Errorf("Should have detected at least one constant, got %v", info.Constants)
	}

	foundSpring := false
	for _, fw := range info.Frameworks {
		if fw == "spring-boot" || fw == "spring" {
			foundSpring = true
		}
	}
	if !foundSpring {
		t.Errorf("Should detect spring framework from imports, got %v", info.Frameworks)
	}

	foundConcurrency := false
	for _, tag := range info.RiskTags {
		if tag == "concurrency" {
			foundConcurrency = true
		}
	}
	if !foundConcurrency {
		t.Errorf("Should detect concurrency risk from java.util.concurrent import, got %v", info.RiskTags)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	content := []byte("Hello, World!")
	password := "testpassword123"

	// Create temp file for encryption
	tmpDir := t.TempDir()
	encPath := filepath.Join(tmpDir, "test.enc")

	// Encrypt
	if err := encryptToFile(content, password, encPath); err != nil {
		t.Fatalf("encryptToFile failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(encPath); os.IsNotExist(err) {
		t.Fatal("Encrypted file should exist")
	}

	// Decrypt
	decrypted, err := DecryptFromFile(encPath, password)
	if err != nil {
		t.Fatalf("DecryptFromFile failed: %v", err)
	}

	if string(decrypted) != string(content) {
		t.Errorf("Decrypted content doesn't match: got %q, want %q", string(decrypted), string(content))
	}

	// Wrong password should fail
	_, err = DecryptFromFile(encPath, "wrongpassword")
	if err == nil {
		t.Error("Decryption with wrong password should fail")
	}
}

func TestIsEncrypted(t *testing.T) {
	if IsEncrypted([]byte("plain text")) {
		t.Error("Plain text should not be detected as encrypted")
	}

	if !IsEncrypted([]byte("COMANDA_ENCRYPTED_V1:somedata")) {
		t.Error("Encrypted prefix should be detected")
	}
}

func TestHashComputation(t *testing.T) {
	config := DefaultConfig()
	manager, _ := NewManager(config, false)

	content := []byte("test content")

	// xxhash (default)
	hash1 := manager.computeHash(content)
	if hash1 == "" {
		t.Error("xxhash should produce a hash")
	}

	// sha256
	manager.config.HashAlgorithm = HashSHA256
	hash2 := manager.computeHash(content)
	if hash2 == "" {
		t.Error("sha256 should produce a hash")
	}

	// Different algorithms should produce different hashes
	if hash1 == hash2 {
		t.Error("xxhash and sha256 should produce different hashes")
	}
}

func TestCombinedIgnoreDirs(t *testing.T) {
	adapters := []Adapter{
		&GoAdapter{},
		&PythonAdapter{},
	}

	dirs := CombinedIgnoreDirs(adapters)
	if len(dirs) == 0 {
		t.Error("Should have combined ignore dirs")
	}

	// Should include both Go and Python ignores
	hasVendor := false
	hasPycache := false
	for _, d := range dirs {
		if d == "vendor" {
			hasVendor = true
		}
		if d == "__pycache__" {
			hasPycache = true
		}
	}

	if !hasVendor {
		t.Error("Should include 'vendor' from Go adapter")
	}
	if !hasPycache {
		t.Error("Should include '__pycache__' from Python adapter")
	}
}

func TestFullGeneration(t *testing.T) {
	// Skip if running in CI without a real repo
	if os.Getenv("CI") != "" {
		t.Skip("Skipping full generation test in CI")
	}

	// Find repo root (go up until we find go.mod)
	root := "."
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		root = filepath.Join(root, "..")
	}

	config := DefaultConfig()
	config.Root = root
	config.MaxOutputKB = 50

	manager, err := NewManager(config, false)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	result, err := manager.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if result.Content == "" {
		t.Error("Generated content should not be empty")
	}

	if result.OutputPath == "" {
		t.Error("Output path should not be empty")
	}

	if result.ContentHash == "" {
		t.Error("Content hash should not be empty")
	}

	// Content should have expected sections
	if !strings.Contains(result.Content, "# ") {
		t.Error("Content should have markdown headers")
	}

	if !strings.Contains(result.Content, "Repository Layout") {
		t.Error("Content should have Repository Layout section")
	}
}

func TestGenerateIncremental(t *testing.T) {
	// Create a temp directory with some Go files
	tmpDir := t.TempDir()

	// Create initial files
	mainGo := filepath.Join(tmpDir, "main.go")
	os.WriteFile(mainGo, []byte(`package main

func main() {
	println("hello")
}
`), 0644)

	utilGo := filepath.Join(tmpDir, "util.go")
	os.WriteFile(utilGo, []byte(`package main

func helper() string {
	return "helper"
}
`), 0644)

	// Create go.mod so it's detected as a Go project
	goMod := filepath.Join(tmpDir, "go.mod")
	os.WriteFile(goMod, []byte("module test\ngo 1.21\n"), 0644)

	// First, generate the initial index
	cfg := DefaultConfig()
	cfg.Root = tmpDir
	cfg.OutputPath = filepath.Join(tmpDir, ".comanda", "index.md")
	cfg.OutputFormat = FormatStructured
	os.MkdirAll(filepath.Join(tmpDir, ".comanda"), 0755)

	manager, err := NewManager(cfg, false)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	initialResult, err := manager.Generate()
	if err != nil {
		t.Fatalf("Initial Generate failed: %v", err)
	}

	if initialResult.FileCount < 2 {
		t.Errorf("Expected at least 2 files, got %d", initialResult.FileCount)
	}

	// Now test incremental update with no changes
	manager2, _ := NewManager(cfg, false)
	result, wasIncremental, err := manager2.GenerateIncremental(initialResult.OutputPath)
	if err != nil {
		t.Fatalf("GenerateIncremental failed: %v", err)
	}

	if !wasIncremental {
		t.Error("Expected incremental update to succeed")
	}
	if result.Updated {
		t.Error("Expected Updated=false when no changes")
	}

	// Now modify a file and test incremental update
	os.WriteFile(mainGo, []byte(`package main

func main() {
	println("hello world") // modified
}
`), 0644)

	manager3, _ := NewManager(cfg, false)
	result2, wasIncremental2, err := manager3.GenerateIncremental(initialResult.OutputPath)
	if err != nil {
		t.Fatalf("GenerateIncremental after modification failed: %v", err)
	}

	if !wasIncremental2 {
		t.Error("Expected incremental update after modification")
	}
	if !result2.Updated {
		t.Error("Expected Updated=true when files changed")
	}
	if result2.ContentHash == initialResult.ContentHash {
		t.Error("Expected different content hash after modification")
	}

	// Test adding a new file
	newFile := filepath.Join(tmpDir, "new.go")
	os.WriteFile(newFile, []byte(`package main

func newFunc() {}
`), 0644)

	manager4, _ := NewManager(cfg, false)
	result3, wasIncremental3, err := manager4.GenerateIncremental(result2.OutputPath)
	if err != nil {
		t.Fatalf("GenerateIncremental after add failed: %v", err)
	}

	if !wasIncremental3 {
		t.Error("Expected incremental update after adding file")
	}
	if !result3.Updated {
		t.Error("Expected Updated=true when file added")
	}
	if result3.FileCount <= result2.FileCount {
		t.Errorf("Expected more files after add, got %d vs %d", result3.FileCount, result2.FileCount)
	}
}

func TestGenerateIncrementalFallback(t *testing.T) {
	// Test that incremental falls back to full generation when metadata missing
	tmpDir := t.TempDir()

	mainGo := filepath.Join(tmpDir, "main.go")
	os.WriteFile(mainGo, []byte("package main\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\ngo 1.21\n"), 0644)

	cfg := DefaultConfig()
	cfg.Root = tmpDir
	cfg.OutputPath = filepath.Join(tmpDir, "index.md")

	// Create a fake index file without metadata
	os.WriteFile(cfg.OutputPath, []byte("# Fake Index\n"), 0644)

	manager, _ := NewManager(cfg, false)
	result, wasIncremental, err := manager.GenerateIncremental(cfg.OutputPath)
	if err != nil {
		t.Fatalf("GenerateIncremental should not fail: %v", err)
	}

	// Should fall back to full generation
	if wasIncremental {
		t.Error("Expected fallback to full generation when metadata missing")
	}
	if !result.Updated {
		t.Error("Expected Updated=true for full regeneration")
	}
}
