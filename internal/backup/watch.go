package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type WatchEvent struct {
	Files        []string    `json:"files"`
	ProcessedDir string      `json:"processed_dir"`
	Result       *SyncResult `json:"result,omitempty"`
	Error        string      `json:"error,omitempty"`
}
type fileStamp struct {
	size     int64
	modified time.Time
}

// Watch waits for two unchanged scans before importing a file. Failed snapshots
// are retried when they change; successful imports are deduplicated by Sync.
func Watch(ctx context.Context, store, folder string, opts SyncOptions, interval time.Duration, emit func(WatchEvent) error) error {
	if emit == nil {
		return errors.New("watch requires an event handler")
	}
	if interval <= 0 {
		return errors.New("watch interval must be positive")
	}
	dir, err := ValidateWatch(store, folder)
	if err != nil {
		return err
	}
	processed := filepath.Join(dir, "procesados")
	if err = os.MkdirAll(processed, 0700); err != nil {
		return err
	}
	observed, failed := map[string]fileStamp{}, map[string]fileStamp{}
	var lastProblem string
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		files, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		fresh := map[string]fileStamp{}
		var ready []string
		for _, file := range files {
			if !file.Type().IsRegular() || !strings.EqualFold(filepath.Ext(file.Name()), ".jwlibrary") {
				continue
			}
			stat, e := file.Info()
			if e != nil {
				return e
			}
			path := filepath.Join(dir, file.Name())
			stamp := fileStamp{stat.Size(), stat.ModTime()}
			fresh[path] = stamp
			if prior, ok := observed[path]; ok && prior == stamp && failed[path] != stamp {
				ready = append(ready, path)
			}
		}
		observed = fresh
		sort.Strings(ready)
		hashes := map[string]string{}
		var valid []string
		for _, path := range ready {
			archive, e := Open(ctx, path)
			if e == nil {
				archive.Close()
				hashes[path], e = backupHash(path)
			}
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				failed[path] = observed[path]
				if err = emit(WatchEvent{Files: []string{path}, ProcessedDir: processed, Error: e.Error()}); err != nil {
					return err
				}
				continue
			}
			valid = append(valid, path)
		}
		if len(valid) > 0 {
			event := WatchEvent{Files: valid, ProcessedDir: processed}
			event.Result, err = Sync(ctx, store, valid, opts)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				event.Error = err.Error()
			} else {
				var problems []string
				for _, path := range valid {
					if e := moveProcessed(path, processed, hashes[path]); e != nil {
						problems = append(problems, e.Error())
					} else {
						delete(observed, path)
						delete(failed, path)
					}
				}
				event.Error = strings.Join(problems, "; ")
			}
			problem := strings.Join(valid, "\n") + event.Error
			if event.Error == "" || problem != lastProblem {
				if err = emit(event); err != nil {
					return err
				}
			}
			if event.Error == "" {
				lastProblem = ""
			} else {
				lastProblem = problem
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func nestedPath(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
func backupHash(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func moveProcessed(input, dir, hash string) error {
	now, err := backupHash(input)
	if err != nil {
		return err
	}
	if now != hash {
		return errors.New("incoming file changed during sync; leaving it for another scan")
	}
	destination := filepath.Join(dir, filepath.Base(input))
	if _, err = os.Stat(destination); err == nil {
		destination = filepath.Join(dir, strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))+"-"+hash+".jwlibrary")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	copied, err := copyBackup(input, destination)
	if errors.Is(err, os.ErrExist) {
		existing, e := backupHash(destination)
		if e != nil {
			return e
		}
		if existing != hash {
			return errors.New("processed destination contains a different backup")
		}
		copied = existing
		err = nil
	}
	if err != nil {
		return err
	}
	if copied != hash {
		os.Remove(destination)
		return errors.New("incoming file changed while moving; original retained")
	}
	final, err := backupHash(input)
	if err != nil {
		return err
	}
	if final != hash {
		return errors.New("incoming file changed before removal; original retained")
	}
	if err = os.Remove(input); err != nil {
		return fmt.Errorf("backup synced but could not remove processed input: %w", err)
	}
	return syncDirectory(dir)
}

// ValidateWatch lets callers reject an invalid watch location before importing arguments.
func ValidateWatch(store, folder string) (string, error) {
	dir, err := filepath.Abs(folder)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(store)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("--watch must name a directory")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		realRoot = root
	} else if err != nil {
		return "", err
	}
	if nestedPath(realRoot, resolved) || nestedPath(resolved, realRoot) {
		return "", errors.New("--watch and --store must be separate directories")
	}
	return dir, nil
}
