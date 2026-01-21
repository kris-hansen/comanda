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
