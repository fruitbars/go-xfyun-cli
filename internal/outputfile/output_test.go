package outputfile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentCommitDoesNotOverwrite(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "result")
	var files [2]*os.File
	for i := range files {
		_, file, err := Prepare(destination, false)
		if err != nil {
			t.Fatal(err)
		}
		files[i] = file
		if _, err := file.WriteString(string(rune('a' + i))); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(file.Name())
	}
	var results [2]error
	var wg sync.WaitGroup
	for i := range files {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i] = Commit(files[i].Name(), destination, false) }(i)
	}
	wg.Wait()
	winner := -1
	for i, err := range results {
		if err == nil {
			if winner != -1 {
				t.Fatal("both commits succeeded")
			}
			winner = i
		}
	}
	if winner == -1 {
		t.Fatalf("neither commit succeeded: %v", results)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != string(rune('a'+winner)) {
		t.Fatalf("output=%q error=%v", data, err)
	}
}

func TestForceCommitPreservesOldFileOnFailure(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "result")
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	absolute, file, err := Prepare(destination, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file.Name()); err != nil {
		t.Fatal(err)
	}
	if err := Commit(file.Name(), absolute, true); err == nil {
		t.Fatal("expected failed commit")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "old" {
		t.Fatalf("old output lost: %q %v", data, err)
	}
}

func TestForceCommitReplacesFile(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "result")
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	absolute, file, err := Prepare(destination, true)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Commit(file.Name(), absolute, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "new" {
		t.Fatalf("output=%q error=%v", data, err)
	}
}

func TestPrepareRejectsDanglingSymlink(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "result")
	if err := os.Symlink(filepath.Join(directory, "missing"), destination); err != nil {
		t.Skip(err)
	}
	if _, file, err := Prepare(destination, false); err == nil {
		file.Close()
		os.Remove(file.Name())
		t.Fatal("dangling symlink must count as an existing output")
	}
}
