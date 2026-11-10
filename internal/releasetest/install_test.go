package releasetest_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const archiveName = "mrimpact_0.0.1_linux_amd64.tar.gz"

const fakeBinary = "#!/bin/sh\necho mrimpact fake\n"

func needTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"sh", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		if _, err := exec.LookPath("shasum"); err != nil {
			t.Skip("neither sha256sum nor shasum available")
		}
	}
}

func makeArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(fakeBinary)
	if err := tw.WriteHeader(&tar.Header{Name: "mrimpact", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// server serves a release v0.0.1. checksums is the body of checksums.txt;
// when nil it is derived from the archive.
func server(t *testing.T, checksums func(archive []byte) string) *httptest.Server {
	t.Helper()
	archive := makeArchive(t)
	sums := fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), archiveName)
	if checksums != nil {
		sums = checksums(archive)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/download/v0.0.1/"+archiveName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/download/v0.0.1/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v0.0.1", http.StatusFound)
	})
	mux.HandleFunc("/tag/v0.0.1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tag page"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// run executes install.sh and returns combined output and the exit error.
func run(t *testing.T, srv *httptest.Server, path string, env []string, args ...string) (string, error) {
	t.Helper()
	script, err := filepath.Abs("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(),
		"MRIMPACT_DOWNLOAD_URL="+srv.URL+"/download",
		"MRIMPACT_LATEST_URL="+srv.URL+"/latest",
		"MRIMPACT_OS=linux", "MRIMPACT_ARCH=amd64",
	)
	if path != "" {
		cmd.Env = append(cmd.Env, "PATH="+path)
	}
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func assertInstalled(t *testing.T, bindir, out string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	want := "mrimpact v0.0.1 installed to " + bindir + "/mrimpact"
	if !strings.Contains(out, want) {
		t.Errorf("output %q lacks %q", out, want)
	}
	fi, statErr := os.Stat(filepath.Join(bindir, "mrimpact"))
	if statErr != nil {
		t.Fatalf("binary not installed: %v", statErr)
	}
	if fi.Mode()&0o111 == 0 {
		t.Errorf("binary not executable: %v", fi.Mode())
	}
	got, _ := os.ReadFile(filepath.Join(bindir, "mrimpact"))
	if string(got) != fakeBinary {
		t.Errorf("binary content = %q", got)
	}
}

func assertNothingInstalled(t *testing.T, bindir string) {
	t.Helper()
	entries, _ := os.ReadDir(bindir)
	if len(entries) != 0 {
		t.Errorf("bindir not empty: %v", entries)
	}
}

func TestInstallScript_InstallsVerified(t *testing.T) {
	needTools(t)
	srv := server(t, nil)
	bindir := filepath.Join(t.TempDir(), "bin")
	out, err := run(t, srv, "", nil, "-b", bindir, "v0.0.1")
	assertInstalled(t, bindir, out, err)
}

func TestInstallScript_VersionWithoutV(t *testing.T) {
	needTools(t)
	srv := server(t, nil)
	bindir := filepath.Join(t.TempDir(), "bin")
	out, err := run(t, srv, "", nil, "-b", bindir, "0.0.1")
	assertInstalled(t, bindir, out, err)
}

func TestInstallScript_ChecksumMismatch(t *testing.T) {
	needTools(t)
	srv := server(t, func([]byte) string {
		return strings.Repeat("0", 64) + "  " + archiveName + "\n"
	})
	bindir := filepath.Join(t.TempDir(), "bin")
	out, err := run(t, srv, "", nil, "-b", bindir, "v0.0.1")
	if err == nil {
		t.Fatalf("expected failure, got success:\n%s", out)
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		t.Errorf("exit = %v, want 1", err)
	}
	if !strings.Contains(out, "checksum") {
		t.Errorf("message does not mention checksum: %q", out)
	}
	assertNothingInstalled(t, bindir)
}

func TestInstallScript_MissingChecksumEntry(t *testing.T) {
	needTools(t)
	srv := server(t, func(a []byte) string {
		return fmt.Sprintf("%x  mrimpact_0.0.1_linux_arm64.tar.gz\n", sha256.Sum256(a))
	})
	bindir := filepath.Join(t.TempDir(), "bin")
	out, err := run(t, srv, "", nil, "-b", bindir, "v0.0.1")
	if err == nil {
		t.Fatalf("expected failure, got success:\n%s", out)
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		t.Errorf("exit = %v, want 1", err)
	}
	if !strings.Contains(out, archiveName) {
		t.Errorf("message does not name the archive: %q", out)
	}
	assertNothingInstalled(t, bindir)
}

func TestInstallScript_Latest(t *testing.T) {
	needTools(t)
	srv := server(t, nil)
	bindir := filepath.Join(t.TempDir(), "bin")
	out, err := run(t, srv, "", nil, "-b", bindir)
	assertInstalled(t, bindir, out, err)
}

func TestInstallScript_LatestWithoutTag(t *testing.T) {
	needTools(t)
	srv := server(t, nil)
	bindir := filepath.Join(t.TempDir(), "bin")
	// /tag/v0.0.1 serves 200 with no Location: no tag can be resolved.
	out, err := run(t, srv, "", []string{"MRIMPACT_LATEST_URL=" + srv.URL + "/tag/v0.0.1"}, "-b", bindir)
	if err == nil {
		t.Fatalf("expected failure:\n%s", out)
	}
	assertNothingInstalled(t, bindir)
}

func TestInstallScript_UnknownArch(t *testing.T) {
	needTools(t)
	srv := server(t, nil)
	bindir := filepath.Join(t.TempDir(), "bin")
	out, err := run(t, srv, "", []string{"MRIMPACT_ARCH=mips"}, "-b", bindir, "v0.0.1")
	if err == nil {
		t.Fatalf("expected failure:\n%s", out)
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		t.Errorf("exit = %v, want 1", err)
	}
	if !strings.Contains(out, "mips") {
		t.Errorf("message does not name the arch: %q", out)
	}
	assertNothingInstalled(t, bindir)
}

// TestInstallScript_WgetOnly runs with a PATH that has wget but no curl.
func TestInstallScript_WgetOnly(t *testing.T) {
	needTools(t)
	wget, err := exec.LookPath("wget")
	if err != nil {
		t.Skip("wget not available")
	}
	dir := t.TempDir()
	tools := []string{"sh", "tar", "gzip", "mktemp", "rm", "mv", "cp", "chmod", "mkdir", "uname",
		"sha256sum", "shasum", "grep", "sed", "awk", "tr", "cut", "head", "tail", "cat", "dirname", "basename", "id", "perl", "env", "unzip"}
	for _, n := range tools {
		if p, err := exec.LookPath(n); err == nil {
			_ = os.Symlink(p, filepath.Join(dir, n))
		}
	}
	_ = os.Symlink(wget, filepath.Join(dir, "wget"))
	srv := server(t, nil)
	bindir := filepath.Join(t.TempDir(), "bin")
	for _, args := range [][]string{{"-b", bindir, "v0.0.1"}, {"-b", bindir}} {
		_ = os.RemoveAll(bindir)
		out, err := run(t, srv, dir, nil, args...)
		assertInstalled(t, bindir, out, err)
	}
}
