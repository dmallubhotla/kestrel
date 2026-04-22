package swoop

import (
	"path/filepath"
	"testing"
)

func TestDetectLayout_ServiceEmbedded(t *testing.T) {
	roots := []Root{
		{Path: filepath.Join("misc", "iac", "live", "dev"), Dir: "misc"},
		{Path: filepath.Join("misc", "iac", "live", "stage"), Dir: "misc"},
		{Path: filepath.Join("misc", "iac", "live", "prod"), Dir: "misc"},
		{Path: filepath.Join("misc", "iac", "live", "ci"), Dir: "misc"},
	}

	layout := DetectLayout(roots)

	if layout.Type != "service" {
		t.Fatalf("expected type=service, got %q", layout.Type)
	}
	if layout.IACDir != filepath.Join("misc", "iac") {
		t.Errorf("expected iac_dir=%q, got %q", filepath.Join("misc", "iac"), layout.IACDir)
	}
	expected := []string{"ci", "dev", "prod", "stage"}
	if len(layout.EnvNames) != len(expected) {
		t.Fatalf("expected %d envs, got %d: %v", len(expected), len(layout.EnvNames), layout.EnvNames)
	}
	for i, name := range expected {
		if layout.EnvNames[i] != name {
			t.Errorf("env[%d] = %q, want %q", i, layout.EnvNames[i], name)
		}
	}
}

func TestDetectLayout_Centralized(t *testing.T) {
	roots := []Root{
		{Path: filepath.Join("dev", "networking", "vpc"), Dir: "dev"},
		{Path: filepath.Join("dev", "data-stores", "rds"), Dir: "dev"},
		{Path: filepath.Join("prd", "us-east-1", "prod", "vpc"), Dir: "prd"},
		{Path: filepath.Join("dr", "networking", "vpc"), Dir: "dr"},
	}

	layout := DetectLayout(roots)

	if layout.Type != "centralized" {
		t.Fatalf("expected type=centralized, got %q", layout.Type)
	}
	if layout.IACDir != "" {
		t.Errorf("expected empty iac_dir, got %q", layout.IACDir)
	}
	expected := []string{"dev", "dr", "prd"}
	if len(layout.EnvNames) != len(expected) {
		t.Fatalf("expected %d envs, got %d: %v", len(expected), len(layout.EnvNames), layout.EnvNames)
	}
	for i, name := range expected {
		if layout.EnvNames[i] != name {
			t.Errorf("env[%d] = %q, want %q", i, layout.EnvNames[i], name)
		}
	}
}

func TestDetectLayout_MixedNotService(t *testing.T) {
	// If some roots have live/ and some don't, it's centralized.
	roots := []Root{
		{Path: filepath.Join("misc", "iac", "live", "dev"), Dir: "misc"},
		{Path: filepath.Join("infra", "networking", "vpc"), Dir: "infra"},
	}

	layout := DetectLayout(roots)
	if layout.Type != "centralized" {
		t.Fatalf("expected type=centralized, got %q", layout.Type)
	}
}

func TestDetectLayout_DifferentPrefixes(t *testing.T) {
	// Two different live/ prefixes — not a clean service layout.
	roots := []Root{
		{Path: filepath.Join("a", "live", "dev"), Dir: "a"},
		{Path: filepath.Join("b", "live", "dev"), Dir: "b"},
	}

	layout := DetectLayout(roots)
	if layout.Type != "centralized" {
		t.Fatalf("expected type=centralized, got %q", layout.Type)
	}
}
