//go:build linux

package bridge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeSaveValidatesBeforeCommitAndRollsBack(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	client := filepath.Join(bin, "tunnel-client")
	mock := `#!/bin/sh
case "$1" in
 init)
  shift
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --profile-dir) dir="$2"; shift 2;;
      --tunnel-id) id="$2"; shift 2;;
      *) shift;;
    esac
  done
  mkdir -p "$dir"
  printf 'tunnel_id: %s\n' "$id" > "$dir/syndirelay.yaml"
  ;;
 doctor)
  [ "$FAKE_DOCTOR_FAIL" = "1" ] && exit 21
  exit 0
  ;;
 *)
  exit 22
  ;;
esac
`
	if err := os.WriteFile(client, []byte(mock), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("SYNDIRELAY_DATA_DIR", filepath.Join(root, "Data"))
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	id1 := "tunnel_1234567890abcdef1234567890abcdef"
	id2 := "tunnel_abcdef1234567890abcdef1234567890"
	key := "test_only_runtime_credential_123456"
	first := mcpConnectionAction{TunnelID: id1, RuntimeAPIKey: key}
	if err := nativeSave(context.Background(), first); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	p, err := localMCPPaths()
	if err != nil {
		t.Fatal(err)
	}
	if actual := readTunnelID(p.Profile); actual != id1 {
		t.Fatalf("id mismatch: %s", actual)
	}
	if actual, err := mcpReadKey(p); err != nil || actual != key {
		t.Fatalf("key mismatch: %s / %v", actual, err)
	}
	original, _ := os.ReadFile(p.Profile)
	revision, _ := os.ReadFile(p.Revision)
	// Saving identical credentials must not invalidate a running revision.
	identical := mcpConnectionAction{TunnelID: id1, ReuseKey: true, Replace: true, PreviousTunnelID: id1}
	if err := nativeSave(context.Background(), identical); err != nil {
		t.Fatalf("idempotent Save: %v", err)
	}
	sameRevision, _ := os.ReadFile(p.Revision)
	if string(revision) != string(sameRevision) {
		t.Fatal("unchanged configuration generated a new revision")
	}
	t.Setenv("FAKE_DOCTOR_FAIL", "1")
	failed := mcpConnectionAction{TunnelID: id2, RuntimeAPIKey: key, Replace: true, PreviousTunnelID: id1}
	if err := nativeSave(context.Background(), failed); err == nil {
		t.Fatal("failed doctor was accepted")
	}
	after, _ := os.ReadFile(p.Profile)
	if string(after) != string(original) {
		t.Fatalf("profile changed after validation failed")
	}
	finalRevision, _ := os.ReadFile(p.Revision)
	if string(revision) != string(finalRevision) {
		t.Fatal("revision changed after failed validation")
	}
	dirs, err := os.ReadDir(p.Private)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if strings.HasPrefix(d.Name(), "pending-") {
			t.Fatal("staged credential directory leaked")
		}
	}
}
