package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Generate renders the wrapper package described by data and writes it to
// outDir, returning the written file paths.
//
// The directory is created when missing. An existing directory that already
// contains entries is refused unless force is true, so the CLI never overwrites
// silently.
func Generate(data TemplateData, outDir string, force bool) ([]string, error) {
	files, err := Render(data)
	if err != nil {
		return nil, err
	}

	if err := checkOutputDir(outDir, force); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return nil, fmt.Errorf("create output directory %q: %w", outDir, err)
	}

	written := make([]string, 0, len(GeneratedFiles))
	for _, name := range GeneratedFiles {
		path := filepath.Join(outDir, name)
		if err := os.WriteFile(path, files[name], 0o600); err != nil {
			return nil, fmt.Errorf("write %q: %w", path, err)
		}
		written = append(written, path)
	}

	return written, nil
}

// checkOutputDir verifies that outDir is usable as an output directory.
func checkOutputDir(outDir string, force bool) error {
	info, err := os.Stat(outDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect output directory %q: %w", outDir, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("output path %q is not a directory", outDir)
	}

	if force {
		return nil
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		return fmt.Errorf("read output directory %q: %w", outDir, err)
	}

	if len(entries) > 0 {
		return fmt.Errorf("output directory %q is not empty, pass --force to write into it", outDir)
	}

	return nil
}
