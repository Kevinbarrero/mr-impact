package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rootFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".mrimpact.yml"), content)
	return dir
}

func TestLoad_NoFileIsDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir(), "")
	if err != nil || !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestLoad_EmptyFileIsDefaults(t *testing.T) {
	cfg, err := Load(rootFile(t, ""), "")
	if err != nil || !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestLoad_CommentsOnlyFileIsDefaults(t *testing.T) {
	cfg, err := Load(rootFile(t, "# nothing\n"), "")
	if err != nil || !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestLoad_Full(t *testing.T) {
	cfg, err := Load("", "testdata/full.yml")
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Critical:        []string{"internal/payment/..."},
		SensitiveAdd:    []string{"github.com/shopspring/decimal.*", "example.com/ledger.(*Ledger).Post"},
		SensitiveRemove: []string{"reflect.*"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestLoad_UnknownKey(t *testing.T) {
	_, err := Load(rootFile(t, "critcal: [x]\n"), "")
	if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "critcal") {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_WrongType(t *testing.T) {
	_, err := Load(rootFile(t, "critical: internal/x\n"), "")
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_BadGlob(t *testing.T) {
	_, err := Load(rootFile(t, `critical: ["internal/[x"]`+"\n"), "")
	if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "internal/[x") {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_ExplicitMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, dir+"/nope.yml")
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_ExplicitPathWins(t *testing.T) {
	dir := rootFile(t, "critical: [root/...]\n")
	explicit := filepath.Join(t.TempDir(), "other.yml")
	write(t, explicit, "critical: [explicit/...]\n")
	cfg, err := Load(dir, explicit)
	if err != nil || !reflect.DeepEqual(cfg.Critical, []string{"explicit/..."}) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}
