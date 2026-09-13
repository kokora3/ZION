package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kokora3/zion/internal/chain"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

const RegistryIndexVersion = 1

var ErrInvalidRegistryQuery = errors.New("invalid registry query")

type ResearchEntry struct {
	ResearchID     string             `json:"research_id"`
	Body           research.EntryBody `json:"body"`
	ProposalID     string             `json:"proposal_id"`
	AdmittedHeight int64              `json:"admitted_height"`
}

type ResourceEntry struct {
	ResourceID     string              `json:"resource_id"`
	Body           resources.EntryBody `json:"body"`
	ProposalID     string              `json:"proposal_id"`
	AdmittedHeight int64               `json:"admitted_height"`
}

type registryDisk struct {
	Version   int                      `json:"version"`
	Research  map[string]ResearchEntry `json:"research"`
	Resources map[string]ResourceEntry `json:"resources"`
}

// RegistryIndex is a replaceable local projection of canonical registry state.
// Its bytes, ordering, and availability are never consensus inputs.
type RegistryIndex struct {
	mu        sync.RWMutex
	path      string
	research  map[string]ResearchEntry
	resources map[string]ResourceEntry
}

func OpenRegistries(path string) (*RegistryIndex, error) {
	if path == "" {
		return nil, fmt.Errorf("registry index path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	result := &RegistryIndex{path: abs, research: map[string]ResearchEntry{}, resources: map[string]ResourceEntry{}}
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil || len(data) > 16<<20 {
		return result, nil
	}
	var disk registryDisk
	if json.Unmarshal(data, &disk) != nil || disk.Version != RegistryIndexVersion || disk.Research == nil || disk.Resources == nil {
		return result, nil
	}
	result.research, result.resources = disk.Research, disk.Resources
	return result, nil
}

func (i *RegistryIndex) Rebuild(state chain.State) error {
	researchEntries := make(map[string]ResearchEntry, len(state.Research))
	for key, value := range state.Research {
		researchEntries[key] = ResearchEntry{ResearchID: key, Body: research.CloneBody(value.Body),
			ProposalID: value.ProposalID.String(), AdmittedHeight: value.AdmittedHeight}
	}
	resourceEntries := make(map[string]ResourceEntry, len(state.Resources))
	for key, value := range state.Resources {
		resourceEntries[key] = ResourceEntry{ResourceID: key, Body: resources.CloneBody(value.Body),
			ProposalID: value.ProposalID.String(), AdmittedHeight: value.AdmittedHeight}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.research, i.resources = researchEntries, resourceEntries
	return i.saveLocked()
}

func (i *RegistryIndex) Research(id string) (ResearchEntry, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	value, ok := i.research[id]
	value.Body = research.CloneBody(value.Body)
	return value, ok
}

func (i *RegistryIndex) Resource(id string) (ResourceEntry, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	value, ok := i.resources[id]
	value.Body = resources.CloneBody(value.Body)
	return value, ok
}

func (i *RegistryIndex) ResearchList(offset, limit int) ([]ResearchEntry, error) {
	if !validPage(offset, limit) {
		return nil, ErrInvalidRegistryQuery
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	items := make([]ResearchEntry, 0, len(i.research))
	for _, value := range i.research {
		items = append(items, value)
	}
	sort.Slice(items, func(a, b int) bool { return items[a].ResearchID < items[b].ResearchID })
	return pageResearch(items, offset, limit), nil
}

func (i *RegistryIndex) ResourceList(offset, limit int) ([]ResourceEntry, error) {
	if !validPage(offset, limit) {
		return nil, ErrInvalidRegistryQuery
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	items := make([]ResourceEntry, 0, len(i.resources))
	for _, value := range i.resources {
		items = append(items, value)
	}
	sort.Slice(items, func(a, b int) bool { return items[a].ResourceID < items[b].ResourceID })
	return pageResource(items, offset, limit), nil
}

func (i *RegistryIndex) ResearchSearch(query string, offset, limit int) ([]ResearchEntry, error) {
	if !validSearch(query, offset, limit) {
		return nil, ErrInvalidRegistryQuery
	}
	want := strings.ToLower(query)
	i.mu.RLock()
	defer i.mu.RUnlock()
	items := []ResearchEntry{}
	for _, value := range i.research {
		haystack := value.Body.Title + "\n" + value.Body.Summary + "\n" + value.Body.ExternalURI
		for _, contributor := range value.Body.Contributors {
			haystack += "\n" + contributor.DisplayName
		}
		for _, external := range value.Body.ExternalIdentifiers {
			haystack += "\n" + string(external.Scheme) + " " + external.Value
		}
		if strings.Contains(strings.ToLower(haystack), want) {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(a, b int) bool { return items[a].ResearchID < items[b].ResearchID })
	return pageResearch(items, offset, limit), nil
}

func (i *RegistryIndex) ResourceSearch(query string, offset, limit int) ([]ResourceEntry, error) {
	if !validSearch(query, offset, limit) {
		return nil, ErrInvalidRegistryQuery
	}
	want := strings.ToLower(query)
	i.mu.RLock()
	defer i.mu.RUnlock()
	items := []ResourceEntry{}
	for _, value := range i.resources {
		haystack := string(value.Body.Kind) + "\n" + value.Body.Name + "\n" + value.Body.Summary + "\n" + value.Body.ExternalURI + "\n" + value.Body.License
		for _, maintainer := range value.Body.Maintainers {
			haystack += "\n" + maintainer.DisplayName
		}
		if strings.Contains(strings.ToLower(haystack), want) {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(a, b int) bool { return items[a].ResourceID < items[b].ResourceID })
	return pageResource(items, offset, limit), nil
}

func (i *RegistryIndex) Counts() (int, int) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.research), len(i.resources)
}

func validPage(offset, limit int) bool { return offset >= 0 && limit >= 1 && limit <= HardQueryLimit }
func validSearch(query string, offset, limit int) bool {
	return query != "" && utf8.ValidString(query) && len([]byte(query)) <= MaxSearchBytes && validPage(offset, limit)
}

func pageResearch(items []ResearchEntry, offset, limit int) []ResearchEntry {
	if offset >= len(items) {
		return []ResearchEntry{}
	}
	result := append([]ResearchEntry(nil), items[offset:min(len(items), offset+limit)]...)
	for position := range result {
		result[position].Body = research.CloneBody(result[position].Body)
	}
	return result
}
func pageResource(items []ResourceEntry, offset, limit int) []ResourceEntry {
	if offset >= len(items) {
		return []ResourceEntry{}
	}
	result := append([]ResourceEntry(nil), items[offset:min(len(items), offset+limit)]...)
	for position := range result {
		result[position].Body = resources.CloneBody(result[position].Body)
	}
	return result
}

func (i *RegistryIndex) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(i.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(registryDisk{Version: RegistryIndexVersion, Research: i.research, Resources: i.resources})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(i.path), ".registry-index-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0o600)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	backup := i.path + ".previous"
	_ = os.Remove(backup)
	if err = os.Rename(i.path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.Rename(name, i.path); err != nil {
		_ = os.Rename(backup, i.path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
