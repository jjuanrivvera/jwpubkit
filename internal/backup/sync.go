package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const masterName = "master.jwlibrary"
const baseName = "base.jwlibrary"

type SyncOptions struct {
	MergeOptions
	HistoryLimit int
}
type SyncStatus struct {
	Store       string         `json:"store"`
	Initialized bool           `json:"initialized"`
	Master      string         `json:"master,omitempty"`
	Base        string         `json:"base,omitempty"`
	MasterDate  string         `json:"master_date,omitempty"`
	LastSync    string         `json:"last_sync,omitempty"`
	Device      string         `json:"device_name,omitempty"`
	Counts      map[string]int `json:"counts,omitempty"`
	History     []string       `json:"history"`
}
type SyncResult struct {
	Status        SyncStatus `json:"status"`
	Reports       []*Report  `json:"reports"`
	Imported      int        `json:"imported"`
	AlreadySynced int        `json:"already_synced"`
}
type syncState struct {
	LastSync string          `json:"last_sync"`
	Previous string          `json:"previous_generation,omitempty"`
	Seen     map[string]bool `json:"seen"`
}
type currentGeneration struct {
	Generation string `json:"generation"`
}

func acquireStore(dir string, create bool) (string, *os.File, error) {
	if strings.TrimSpace(dir) == "" {
		return "", nil, errors.New("backup store is empty; configure backup_store or use --store")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, err
	}
	if create {
		if err = os.MkdirAll(abs, 0700); err != nil {
			return "", nil, err
		}
	}
	f, err := os.OpenFile(filepath.Join(abs, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return abs, nil, err
	}
	if err = lockStore(f); err != nil {
		f.Close()
		return abs, nil, fmt.Errorf("backup store is busy: %w", err)
	}
	return abs, f, nil
}

func readGeneration(dir string) (string, syncState, error) {
	var state syncState
	b, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if errors.Is(err, os.ErrNotExist) {
		for _, name := range []string{masterName, baseName} {
			if _, e := os.Stat(filepath.Join(dir, name)); e == nil {
				return "", state, errors.New("store has backup exports without current.json; move them outside --store before initializing")
			} else if !errors.Is(e, os.ErrNotExist) {
				return "", state, e
			}
		}
		return "", syncState{Seen: map[string]bool{}}, nil
	}
	if err != nil {
		return "", state, err
	}
	var ptr currentGeneration
	if err = json.Unmarshal(b, &ptr); err != nil {
		return "", state, fmt.Errorf("invalid backup store pointer: %w", err)
	}
	if len(ptr.Generation) != 32 {
		return "", state, errors.New("invalid backup generation ID")
	}
	if _, err = hex.DecodeString(ptr.Generation); err != nil {
		return "", state, errors.New("invalid backup generation ID")
	}
	gen := filepath.Join(dir, "history", ptr.Generation)
	b, err = os.ReadFile(filepath.Join(gen, "state.json"))
	if err != nil {
		return "", state, err
	}
	if err = json.Unmarshal(b, &state); err != nil {
		return "", state, err
	}
	if state.Seen == nil {
		state.Seen = map[string]bool{}
	}
	return gen, state, nil
}

// Status also repairs stable exports if a process stopped after committing a generation.
func Status(ctx context.Context, dir string) (SyncStatus, error) {
	abs, lock, err := acquireStore(dir, false)
	if errors.Is(err, os.ErrNotExist) {
		return SyncStatus{Store: abs, History: []string{}}, nil
	}
	if err != nil {
		return SyncStatus{}, err
	}
	defer lock.Close()
	gen, state, err := readGeneration(abs)
	if err != nil {
		return SyncStatus{}, err
	}
	if gen != "" {
		if err = exportGeneration(abs, gen); err != nil {
			return SyncStatus{}, err
		}
	}
	return inspectStore(ctx, abs, gen, state)
}

func inspectStore(ctx context.Context, dir, gen string, state syncState) (SyncStatus, error) {
	s := SyncStatus{Store: dir, History: []string{}}
	if gen == "" {
		return s, nil
	}
	a, err := Open(ctx, filepath.Join(gen, masterName))
	if err != nil {
		return s, err
	}
	defer a.Close()
	i, err := a.Inspect(ctx)
	if err != nil {
		return s, err
	}
	s.Initialized = true
	s.Master = filepath.Join(dir, masterName)
	s.Base = filepath.Join(dir, baseName)
	s.LastSync = state.LastSync
	s.MasterDate = i.Manifest.Backup.LastModified
	s.Device = i.Manifest.Backup.Device
	s.Counts = i.Counts
	previous, err := previousGenerations(dir, gen, state)
	if err != nil {
		return s, err
	}
	for _, prior := range previous {
		s.History = append(s.History, filepath.Join(prior, masterName))
	}

	return s, nil
}

// Sync commits an entire batch against one stored ancestor. The next base is the
// resulting master, so callers must distribute that master before the next batch.
func Sync(ctx context.Context, dir string, inputs []string, opts SyncOptions) (*SyncResult, error) {
	if len(inputs) == 0 {
		return nil, errors.New("sync requires at least one incoming backup")
	}
	if opts.HistoryLimit < 0 {
		return nil, errors.New("--history-limit cannot be negative")
	}
	if opts.DryRun {
		return nil, errors.New("sync does not accept --dry-run; preview with backup merge --base")
	}
	abs, lock, err := acquireStore(dir, true)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err = validateSyncPreferences(abs, inputs, opts); err != nil {
		return nil, err
	}
	gen, state, err := readGeneration(abs)
	if err != nil {
		return nil, err
	}
	if gen != "" {
		if err = exportGeneration(abs, gen); err != nil {
			return nil, err
		}
	}
	work, err := os.MkdirTemp(abs, ".sync-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	result := &SyncResult{Reports: []*Report{}}
	current, base := "", ""
	var dates [2]time.Time
	if gen != "" {
		current = filepath.Join(gen, masterName)
		base = filepath.Join(gen, baseName)
		a, e := Open(ctx, current)
		if e != nil {
			return nil, e
		}
		stamp, e := time.Parse(time.RFC3339, a.Manifest.Backup.LastModified)
		a.Close()
		if e != nil {
			return nil, e
		}
		dates = [2]time.Time{stamp, stamp}
	}
	if opts.Base != "" {
		base = opts.Base
	}
	device := opts.Device
	if device == "" {
		device = "pubkit master"
	}
	for idx, input := range inputs {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		inputAbs, e := filepath.Abs(input)
		if e != nil {
			return nil, e
		}
		if inputAbs == filepath.Join(abs, masterName) || inputAbs == filepath.Join(abs, baseName) {
			return nil, errors.New("incoming backup cannot be the store's master or base")
		}
		snapshot := filepath.Join(work, fmt.Sprintf("incoming-%d.jwlibrary", idx))
		hash, e := copyBackup(input, snapshot)
		if e != nil {
			return nil, e
		}
		if state.Seen[hash] {
			result.AlreadySynced++
			continue
		}
		incomingArchive, e := Open(ctx, snapshot)
		if e != nil {
			return nil, e
		}
		stamp, e := time.Parse(time.RFC3339, incomingArchive.Manifest.Backup.LastModified)
		incomingArchive.Close()
		if e != nil {
			return nil, fmt.Errorf("incoming backup date: %w", e)
		}
		output := filepath.Join(work, fmt.Sprintf("merged-%d.jwlibrary", idx))
		if current == "" {
			a, e := Open(ctx, snapshot)
			if e != nil {
				return nil, e
			}
			e = a.Save(ctx, output, device)
			a.Close()
			if e != nil {
				return nil, e
			}
		} else {
			mergeOpts := opts.MergeOptions
			mergeOpts.Base = base
			mergeOpts.Device = device
			mergeOpts.sourceTimes = map[string][2]time.Time{current: dates}
			mergeOpts.Prefer = syncPreference(opts.Prefer, current, snapshot, inputAbs, abs)
			mergeOpts.TablePrefer = map[string]string{}
			for table, pref := range opts.TablePrefer {
				mergeOpts.TablePrefer[table] = syncPreference(pref, current, snapshot, inputAbs, abs)
			}
			if opts.Resolve != nil {
				mergeOpts.Resolve = func(c Conflict, a, b map[string]any) (map[string]any, error) {
					switch c.Winner {
					case snapshot:
						c.Winner = inputAbs
					case current:
						c.Winner = filepath.Join(abs, masterName)
					}
					switch c.Other {
					case snapshot:
						c.Other = inputAbs
					case current:
						c.Other = filepath.Join(abs, masterName)
					}
					return opts.Resolve(c, a, b)
				}
			}
			report, e := Merge(ctx, []string{current, snapshot}, output, mergeOpts)
			if e != nil {
				return nil, e
			}
			// Reports identify the original incoming path instead of a disposable copy.
			relabelReport(report, snapshot, inputAbs)
			relabelReport(report, current, filepath.Join(abs, masterName))
			result.Reports = append(result.Reports, report)
		}
		if current == "" {
			dates = [2]time.Time{stamp, stamp}
		} else {
			if stamp.Before(dates[0]) {
				dates[0] = stamp
			}
			if stamp.After(dates[1]) {
				dates[1] = stamp
			}
		}
		state.Seen[hash] = true
		result.Imported++
		current = output
	}
	if result.Imported == 0 {
		if gen != "" {
			if err = rotateHistory(filepath.Join(abs, "history"), filepath.Base(gen), opts.HistoryLimit); err != nil {
				return nil, err
			}
		}
		result.Status, err = inspectStore(ctx, abs, gen, state)
		return result, err
	}
	var idBytes [16]byte
	if _, err = rand.Read(idBytes[:]); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(idBytes[:])
	stage := filepath.Join(work, "generation")
	if err = os.Mkdir(stage, 0700); err != nil {
		return nil, err
	}
	finalArchive, err := Open(ctx, current)
	if err != nil {
		return nil, err
	}
	err = finalArchive.Save(ctx, filepath.Join(stage, masterName), device)
	finalArchive.Close()
	if err != nil {
		return nil, err
	}
	if _, err = copyBackup(filepath.Join(stage, masterName), filepath.Join(stage, baseName)); err != nil {
		return nil, err
	}

	if gen != "" {
		state.Previous = filepath.Base(gen)
	}
	state.LastSync = time.Now().UTC().Format(time.RFC3339Nano)
	if err = writeJSONFile(filepath.Join(stage, "state.json"), state); err != nil {
		return nil, err
	}
	if err = syncDirectory(stage); err != nil {
		return nil, err
	}
	history := filepath.Join(abs, "history")
	if err = os.MkdirAll(history, 0700); err != nil {
		return nil, err
	}
	next := filepath.Join(history, id)
	if err = os.Rename(stage, next); err != nil {
		return nil, err
	}
	if err = syncDirectory(history); err != nil {
		return nil, err
	}
	ptr := filepath.Join(work, "current.json")
	if err = writeJSONFile(ptr, currentGeneration{Generation: id}); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = os.Rename(ptr, filepath.Join(abs, "current.json")); err != nil {
		return nil, err
	}
	if err = syncDirectory(abs); err != nil {
		return nil, err
	}
	// The pointer is the commit boundary: master, base and bookkeeping always
	// belong to the same generation, even if exporting stable filenames is interrupted.
	if err = exportGeneration(abs, next); err != nil {
		return nil, fmt.Errorf("sync committed; run backup status to repair stable exports: %w", err)
	}
	if err = rotateHistory(history, id, opts.HistoryLimit); err != nil {
		return nil, err
	}
	result.Status, err = inspectStore(ctx, abs, next, state)
	return result, err
}

func syncPreference(pref, current, incoming, original, dir string) string {
	switch pref {
	case "master":
		return current
	case "incoming":
		return incoming
	case "", "newest", "oldest":
		return pref
	}
	abs, err := filepath.Abs(pref)
	if err != nil {
		return pref
	}
	if abs == original {
		return incoming
	}
	if abs == filepath.Join(dir, masterName) {
		return current
	}
	return current
}

func relabelReport(r *Report, old, name string) {
	for _, m := range []map[string]map[string]int{r.Before, r.Added} {
		if v, ok := m[old]; ok {
			m[name] = v
			delete(m, old)
		}
	}
	if v, ok := r.RenamedMedia[old]; ok {
		r.RenamedMedia[name] = v
		delete(r.RenamedMedia, old)
	}
	for i := range r.Conflicts {
		if r.Conflicts[i].Winner == old {
			r.Conflicts[i].Winner = name
		}
		if r.Conflicts[i].Other == old {
			r.Conflicts[i].Other = name
		}
	}
	for i := range r.Deletions {
		for j, path := range r.Deletions[i].DeletedIn {
			if path == old {
				r.Deletions[i].DeletedIn[j] = name
			}
		}
	}
	for i := range r.Overlaps {
		if r.Overlaps[i].Source == old {
			r.Overlaps[i].Source = name
		}
	}
}

func copyBackup(src, dest string) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, hash), f)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	return hex.EncodeToString(hash.Sum(nil)), err
}

func writeJSONFile(name string, value any) error {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = json.NewEncoder(f).Encode(value); err != nil {
		return err
	}
	return f.Sync()
}

// Stable exports are separate files so external readers cannot modify retained versions.
func exportGeneration(dir, gen string) error {
	for _, name := range []string{masterName, baseName} {
		src, dest := filepath.Join(gen, name), filepath.Join(dir, name)
		expected, err := backupHash(src)
		if err != nil {
			return err
		}
		actual, err := backupHash(dest)
		if err == nil && actual == expected {
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		tmp, err := os.CreateTemp(dir, ".export-*")
		if err != nil {
			return err
		}
		temp := tmp.Name()
		if err = tmp.Close(); err != nil {
			return err
		}
		if err = os.Remove(temp); err != nil {
			return err
		}
		copied, err := copyBackup(src, temp)
		if err != nil {
			os.Remove(temp)
			return err
		}
		if copied != expected {
			os.Remove(temp)
			return errors.New("generation changed while exporting")
		}
		if err = os.Rename(temp, dest); err != nil {
			os.Remove(temp)
			return err
		}
	}
	return syncDirectory(dir)
}

func previousGenerations(dir, gen string, state syncState) ([]string, error) {
	var out []string
	seen := map[string]bool{filepath.Base(gen): true}
	for id := state.Previous; id != ""; id = state.Previous {
		if len(id) != 32 {
			return nil, errors.New("invalid previous generation ID")
		}
		if _, err := hex.DecodeString(id); err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, errors.New("backup history contains a cycle")
		}
		seen[id] = true
		prior := filepath.Join(dir, "history", id)
		b, err := os.ReadFile(filepath.Join(prior, "state.json"))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return nil, err
		}
		state = syncState{}
		if err = json.Unmarshal(b, &state); err != nil {
			return nil, err
		}
		out = append(out, prior)
	}
	return out, nil
}

func rotateHistory(history, current string, limit int) error {
	dir := filepath.Dir(history)
	gen, state, err := readGeneration(dir)
	if err != nil {
		return err
	}
	previous, err := previousGenerations(dir, gen, state)
	if err != nil {
		return err
	}
	keep := map[string]bool{current: true}
	for i, prior := range previous {
		if i >= limit {
			break
		}
		keep[filepath.Base(prior)] = true
	}
	entries, err := os.ReadDir(history)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || keep[e.Name()] {
			continue
		}
		if len(e.Name()) != 32 {
			continue
		}
		if _, err = hex.DecodeString(e.Name()); err != nil {
			continue
		}
		if err = os.RemoveAll(filepath.Join(history, e.Name())); err != nil {
			return err
		}
	}
	return syncDirectory(history)
}

func validateSyncPreferences(dir string, inputs []string, opts SyncOptions) error {
	allowed := map[string]bool{filepath.Join(dir, masterName): true}
	for _, input := range inputs {
		abs, err := filepath.Abs(input)
		if err != nil {
			return err
		}
		allowed[abs] = true
	}
	preferences := []string{opts.Prefer}
	for _, pref := range opts.TablePrefer {
		preferences = append(preferences, pref)
	}
	for _, pref := range preferences {
		switch pref {
		case "", "master", "incoming", "newest", "oldest":
			continue
		}
		abs, err := filepath.Abs(pref)
		if err != nil {
			return err
		}
		if !allowed[abs] {
			return fmt.Errorf("preferred file %q is not an incoming backup or the master", pref)
		}
	}
	return nil
}
