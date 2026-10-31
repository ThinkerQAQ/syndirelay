package site

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type AssembleResult struct {
	SourceRoot     string
	TargetContent  string
	MediaCopied    bool
	ManifestCopied bool
}

func Assemble(engineRoot, contentRoot string) (AssembleResult, error) {
	sourceContent := filepath.Join(contentRoot, "src", "content")
	targetContent := filepath.Join(engineRoot, "src", "content")
	if info, err := os.Stat(sourceContent); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("content source is not a directory")
		}
		return AssembleResult{}, err
	}

	if err := replaceDirectory(sourceContent, targetContent); err != nil {
		return AssembleResult{}, err
	}

	result := AssembleResult{SourceRoot: contentRoot, TargetContent: targetContent}
	sourceMedia := filepath.Join(contentRoot, "public", "media")
	targetMedia := filepath.Join(engineRoot, "public", "media")
	if info, err := os.Stat(sourceMedia); err == nil && info.IsDir() {
		if err := replaceDirectory(sourceMedia, targetMedia); err != nil {
			return AssembleResult{}, err
		}
		result.MediaCopied = true
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return AssembleResult{}, err
	}

	sourceManifest := filepath.Join(contentRoot, "src", "data", "content-manifest.json")
	targetManifest := filepath.Join(engineRoot, "src", "data", "content-manifest.json")
	if err := os.Remove(targetManifest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return AssembleResult{}, err
	}
	if info, err := os.Stat(sourceManifest); err == nil && !info.IsDir() {
		if err := copyFile(sourceManifest, targetManifest, info.Mode()); err != nil {
			return AssembleResult{}, err
		}
		result.ManifestCopied = true
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return AssembleResult{}, err
	}

	return result, nil
}

func replaceDirectory(source, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(target, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		if !entry.Type().IsRegular() {
			return errors.New("content assembly does not copy non-regular files: " + path)
		}
		return copyFile(path, destination, info.Mode())
	})
}

func copyFile(source, target string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
