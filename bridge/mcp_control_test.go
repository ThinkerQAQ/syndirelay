package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPAutoConnectDefaultsAndPersistence(t *testing.T) {
	p := nativeMCPPaths{Private: t.TempDir()}
	if !mcpAutoConnectEnabled(p) {
		t.Fatal("existing installations must default to auto-connect")
	}
	path := mcpAutoConnectPath(p)
	if err := privateWrite(path, []byte(`{"auto_connect":false}`)); err != nil {
		t.Fatal(err)
	}
	if mcpAutoConnectEnabled(p) {
		t.Fatal("disabled preference was ignored")
	}
	if err := privateWrite(path, []byte(`{"auto_connect":true}`)); err != nil {
		t.Fatal(err)
	}
	if !mcpAutoConnectEnabled(p) {
		t.Fatal("enabled preference was ignored")
	}
	if err := privateWrite(path, []byte("invalid-data")); err != nil {
		t.Fatal(err)
	}
	if mcpAutoConnectEnabled(p) {
		t.Fatal("invalid preference must fail closed")
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatal("preference file invalid")
	}
	if filepath.Dir(path) != p.Private {
		t.Fatal("preferences escaped private directory")
	}
}
