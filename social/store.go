package social

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var storeMu sync.Mutex

func Load(root, id string) (Draft, error) {
	path, err := SafeFile(root, id)
	if err != nil {
		return Draft{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Draft{}, err
	}
	var d Draft
	if err = json.Unmarshal(raw, &d); err != nil {
		return Draft{}, err
	}
	if err = d.Validate(); err != nil {
		return Draft{}, err
	}
	if d.Revision != d.Hash() {
		return Draft{}, errors.New("social draft integrity check failed")
	}
	return d, nil
}
func Save(root string, d Draft) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	if err := d.Validate(); err != nil {
		return err
	}
	if d.Revision == "" {
		d.Revision = d.Hash()
	}
	if d.Revision != d.Hash() {
		return errors.New("content changed; recalculate revision before saving")
	}
	path, err := SafeFile(root, d.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".social-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
func Create(root string, input DraftInput) (Draft, error) {
	draft := Draft{DraftInput: input, Remote: map[string]Remote{}}
	draft.Revision = draft.Hash()
	if err := draft.Validate(); err != nil {
		return Draft{}, err
	}
	path, _ := SafeFile(root, input.ID)
	if _, err := os.Stat(path); err == nil {
		return Draft{}, fmt.Errorf("draft %q already exists", input.ID)
	} else if !os.IsNotExist(err) {
		return Draft{}, err
	}
	if err := Save(root, draft); err != nil {
		return Draft{}, err
	}
	return draft, nil
}
func List(root string) ([]Draft, error) {
	dir := filepath.Join(root, "social")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Draft{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Draft{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		d, err := Load(root, id)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
