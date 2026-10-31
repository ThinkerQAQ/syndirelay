package bridge

import (
	"runtime"
	"strings"
	"testing"
)

func TestNodeVersionSupportsProxy(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"22.20.0", false},
		{"22.21.0", true},
		{"23.11.0", false},
		{"24.0.0", true},
		{"25.1.0", true},
	}
	for _, test := range cases {
		if got := nodeVersionSupportsProxy(test.version); got != test.want {
			t.Fatalf("nodeVersionSupportsProxy(%q) = %v, want %v", test.version, got, test.want)
		}
	}
}

func TestWingetArgsCarryGlobalProxy(t *testing.T) {
	config := bridgeConfig{ProxyEnabled: true, ProxyHost: "127.0.0.1", ProxyPort: 7890}
	args, err := wingetArgs(config, "install", "OpenJS.NodeJS.LTS")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, value := range []string{
		"install",
		"--id OpenJS.NodeJS.LTS",
		"--exact",
		"--silent",
		"--proxy http://127.0.0.1:7890",
	} {
		if !strings.Contains(joined, value) {
			t.Fatalf("winget args %q missing %q", joined, value)
		}
	}
}

func TestJavaWingetPackageMappingDoesNotSwitchVendor(t *testing.T) {
	cases := []struct {
		vendor string
		major  int
		want   string
	}{
		{"Eclipse Adoptium", 21, "EclipseAdoptium.Temurin.21.JDK"},
		{"Microsoft", 21, "Microsoft.OpenJDK.21"},
		{"Oracle Corporation", 21, "Oracle.JDK.21"},
		{"Azul Systems, Inc.", 17, "Azul.Zulu.17.JDK"},
	}
	for _, test := range cases {
		got, err := javaWingetPackageForVendor(test.vendor, test.major)
		if err != nil {
			t.Fatalf("%s: %v", test.vendor, err)
		}
		if got != test.want {
			t.Fatalf("%s package = %q, want %q", test.vendor, got, test.want)
		}
	}
	if _, err := javaWingetPackageForVendor("JetBrains s.r.o.", 21); err == nil {
		t.Fatal("unknown Java vendor should not be switched automatically")
	}
}

func TestDependencyUpdateActionsAreExposed(t *testing.T) {
	config := defaultBridgeConfig()
	seen := map[string]bool{}
	for _, tool := range toolRegistry(config) {
		if tool.Name != "node" && tool.Name != "npm" && tool.Name != "git" && tool.Name != "java" {
			continue
		}
		for _, action := range tool.Actions {
			if action.ID == "update" {
				seen[tool.Name] = true
			}
		}
	}
	for _, name := range []string{"node", "npm", "git", "java"} {
		if !seen[name] {
			t.Fatalf("%s update action missing", name)
		}
	}
}

func TestUpdateWithWingetFailsClosedOutsideWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows fail-closed behavior")
	}
	if _, err := updateWithWinget(t.Context(), defaultBridgeConfig(), "Git.Git"); err == nil {
		t.Fatal("non-Windows automatic WinGet update unexpectedly succeeded")
	}
}
