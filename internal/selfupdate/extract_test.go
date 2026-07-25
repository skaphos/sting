// SPDX-License-Identifier: MIT

package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"testing"
)

// tarGz builds a gzipped tar containing the given entries.
func tarGz(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, data := range entries {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("writing tar header: %v", err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("writing tar entry: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("closing gzip: %v", err)
	}
	return buf.Bytes()
}

func zipArchive(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("creating zip entry: %v", err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("writing zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf.Bytes()
}

// TestExtractFromTarGz mirrors the real archive layout: the binary alongside
// the license and notice files GoReleaser bundles.
func TestExtractFromTarGz(t *testing.T) {
	want := []byte("the sting binary")
	archive := tarGz(t, map[string][]byte{
		"LICENSE":                 []byte("MIT"),
		"THIRD_PARTY_NOTICES.md":  []byte("notices"),
		"sting":                   want,
		"third_party_licenses/go": []byte("x"),
	})

	got, err := ExtractBinary("sting_1.0.0_linux_amd64.tar.gz", archive)
	if err != nil {
		t.Fatalf("ExtractBinary() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}
}

func TestExtractFromZip(t *testing.T) {
	want := []byte("the sting binary for windows")
	archive := zipArchive(t, map[string][]byte{
		"LICENSE":   []byte("MIT"),
		"sting.exe": want,
	})

	got, err := ExtractBinary("sting_1.0.0_windows_amd64.zip", archive)
	if err != nil {
		t.Fatalf("ExtractBinary() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}
}

func TestExtractMissingBinary(t *testing.T) {
	t.Run("tar.gz", func(t *testing.T) {
		archive := tarGz(t, map[string][]byte{"LICENSE": []byte("MIT")})
		if _, err := ExtractBinary("sting_1.0.0_linux_amd64.tar.gz", archive); !errors.Is(err, ErrExtract) {
			t.Errorf("error = %v, want ErrExtract", err)
		}
	})

	t.Run("zip", func(t *testing.T) {
		archive := zipArchive(t, map[string][]byte{"LICENSE": []byte("MIT")})
		if _, err := ExtractBinary("sting_1.0.0_windows_amd64.zip", archive); !errors.Is(err, ErrExtract) {
			t.Errorf("error = %v, want ErrExtract", err)
		}
	})
}

func TestExtractCorruptArchive(t *testing.T) {
	for _, tc := range []struct{ name, asset string }{
		{"tar.gz", "sting_1.0.0_linux_amd64.tar.gz"},
		{"zip", "sting_1.0.0_windows_amd64.zip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ExtractBinary(tc.asset, []byte("not an archive")); !errors.Is(err, ErrExtract) {
				t.Errorf("error = %v, want ErrExtract", err)
			}
		})
	}
}

// TestExtractIgnoresDirectoryEntries guards against a directory named "sting"
// being mistaken for the binary.
func TestExtractIgnoresDirectoryEntries(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "sting/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatalf("writing dir header: %v", err)
	}
	want := []byte("real binary")
	if err := tw.WriteHeader(&tar.Header{
		Name: "sting", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(want)),
	}); err != nil {
		t.Fatalf("writing file header: %v", err)
	}
	if _, err := tw.Write(want); err != nil {
		t.Fatalf("writing entry: %v", err)
	}
	_ = tw.Close()
	_ = gz.Close()

	got, err := ExtractBinary("sting_1.0.0_linux_amd64.tar.gz", buf.Bytes())
	if err != nil {
		t.Fatalf("ExtractBinary() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}
}

func TestIsBinaryEntry(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"sting", true},
		{"sting.exe", true},
		{"./sting", true},
		{"LICENSE", false},
		{"sting.sbom.json", false},
		{"stinger", false},
	} {
		if got := isBinaryEntry(tt.name); got != tt.want {
			t.Errorf("isBinaryEntry(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
