package ifasr

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveTaskProtectsIdentifiers(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "task.json")
	task := TaskFile{Version: 1, Variant: VariantLLM, Parts: []TaskPart{{Index: 1, OrderID: "order", SignatureRandom: "random"}}}
	if err := SaveTask(path, task, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("task file mode = %o, want 600", info.Mode().Perm())
	}
	if err := SaveTask(path, task, false); err == nil {
		t.Fatal("expected replacement rejection")
	}
	if err := SaveTask(path, task, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTask(path)
	if err != nil || len(loaded.Parts) != 1 || loaded.Parts[0].OrderID != "order" {
		t.Fatalf("loaded task = %+v, err = %v", loaded, err)
	}
}
