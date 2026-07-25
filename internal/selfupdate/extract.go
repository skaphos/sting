// SPDX-License-Identifier: MIT

package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
)

// ErrExtract reports that the verified archive did not contain the binary.
var ErrExtract = errors.New("extracting binary")

// maxBinaryBytes bounds decompression so a malformed or hostile archive cannot
// exhaust memory. The archive itself is already verified by the time this runs,
// so this guards against corruption rather than an attacker.
const maxBinaryBytes = 512 << 20

// ExtractBinary pulls the sting executable out of a release archive. Release
// archives are tar.gz everywhere except Windows, which uses zip.
func ExtractBinary(assetName string, archive []byte) ([]byte, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractZip(archive)
	}
	return extractTarGz(archive)
}

// binaryNames are the entry names that count as the executable.
var binaryNames = map[string]bool{"sting": true, "sting.exe": true}

func isBinaryEntry(name string) bool {
	// Archives store the binary at the root, but match on the base name so
	// a layout change does not silently break updates.
	return binaryNames[path.Base(filepath.ToSlash(name))]
}

func extractTarGz(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("%w: opening gzip stream: %w", ErrExtract, err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: reading tar stream: %w", ErrExtract, err)
		}
		if hdr.Typeflag != tar.TypeReg || !isBinaryEntry(hdr.Name) {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxBinaryBytes))
		if err != nil {
			return nil, fmt.Errorf("%w: reading binary from archive: %w", ErrExtract, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%w: no sting binary found in the archive", ErrExtract)
}

func extractZip(archive []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("%w: opening zip archive: %w", ErrExtract, err)
	}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !isBinaryEntry(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: opening binary in archive: %w", ErrExtract, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxBinaryBytes))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: reading binary from archive: %w", ErrExtract, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%w: no sting binary found in the archive", ErrExtract)
}
