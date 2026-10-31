package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A release requires an explicit reviewed and version-pinned license manifest
// for every Go require and every npm lockfile dependency, including optional
// platform-specific npm packages that are absent from the build host.
type licenseDependency struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	License   string `json:"license"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Reviewed  bool   `json:"reviewed"`
}
type licenseManifest struct {
	Schema       int                 `json:"schema"`
	Dependencies []licenseDependency `json:"dependencies"`
}

func requiredGoLicenses(goMod []byte) (map[string]string, error) {
	entries := map[string]string{}
	inRequire := false
	for _, line := range strings.Split(string(goMod), "\n") {
		line = strings.TrimSpace(line)
		if line == "require (" {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		if !inRequire && !strings.HasPrefix(line, "require ") {
			continue
		}
		if strings.HasPrefix(line, "require ") {
			line = strings.TrimPrefix(line, "require ")
		}
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 || !strings.HasPrefix(parts[1], "v") {
			return nil, fmt.Errorf("unrecognized go.mod require line: %q", line)
		}
		if previous, found := entries[parts[0]]; found && previous != parts[1] {
			return nil, fmt.Errorf("conflicting Go requirement: %s", parts[0])
		}
		entries[parts[0]] = parts[1]
	}
	if len(entries) == 0 {
		return nil, errors.New("no pinned Go dependencies found")
	}
	return entries, nil
}
func requiredNpmLicenses(lock []byte) (map[string]string, error) {
	var document struct {
		LockfileVersion int `json:"lockfileVersion"`
		Packages        map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(lock, &document); err != nil {
		return nil, fmt.Errorf("parse npm package lock: %w", err)
	}
	if document.LockfileVersion < 2 || len(document.Packages) < 2 {
		return nil, errors.New("npm lockfile missing supported package metadata")
	}
	entries := map[string]string{}
	for name, pkg := range document.Packages {
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "node_modules/") || strings.TrimSpace(pkg.Version) == "" {
			return nil, fmt.Errorf("invalid npm dependency lock entry: %q", name)
		}
		entries[name] = pkg.Version
	}
	return entries, nil
}
func validateReleaseLicenses(root, goModFile, packageLockFile string) error {
	goMod, err := os.ReadFile(goModFile)
	if err != nil {
		return err
	}
	lock, err := os.ReadFile(packageLockFile)
	if err != nil {
		return err
	}
	expectedGo, err := requiredGoLicenses(goMod)
	if err != nil {
		return err
	}
	expectedNpm, err := requiredNpmLicenses(lock)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "MANIFEST.json"))
	if err != nil {
		return fmt.Errorf("reviewed third-party MANIFEST.json required: %w", err)
	}
	var manifest licenseManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("invalid license manifest: %w", err)
	}
	if manifest.Schema != 1 {
		return errors.New("unsupported release license manifest schema")
	}
	required := map[string]string{}
	for name, version := range expectedGo {
		required["go:"+name] = version
	}
	for name, version := range expectedNpm {
		required["npm:"+name] = version
	}
	reviewed := map[string]bool{}
	files := map[string]bool{}
	for _, entry := range manifest.Dependencies {
		key := entry.Ecosystem + ":" + entry.Name
		version, expected := required[key]
		if !expected {
			return fmt.Errorf("untracked dependency in release license manifest: %s", key)
		}
		if reviewed[key] {
			return fmt.Errorf("duplicate license entry: %s", key)
		}
		if version != entry.Version {
			return fmt.Errorf("stale license version: %s (expected %s)", key, version)
		}
		if !entry.Reviewed || strings.TrimSpace(entry.License) == "" {
			return fmt.Errorf("dependency license not manually reviewed: %s", key)
		}
		cleaned := filepath.Clean(filepath.FromSlash(entry.File))
		if cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || !strings.HasPrefix(filepath.ToSlash(cleaned), entry.Ecosystem+"/") {
			return fmt.Errorf("unsafe license file path: %s", key)
		}
		if files[cleaned] {
			return fmt.Errorf("license file reused by multiple dependencies: %s", entry.File)
		}
		files[cleaned] = true
		target := filepath.Join(root, cleaned)
		info, err := os.Lstat(target)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 25 {
			return fmt.Errorf("missing or invalid license text: %s", key)
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		actual := sha256.Sum256(content)
		if hex.EncodeToString(actual[:]) != strings.ToLower(entry.SHA256) {
			return fmt.Errorf("license text hash mismatch: %s", key)
		}
		reviewed[key] = true
	}
	for key := range required {
		if !reviewed[key] {
			return fmt.Errorf("missing reviewed license for required dependency: %s", key)
		}
	}
	return nil
}
