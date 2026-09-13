package node

import (
	"context"

	"github.com/kokora3/zion/internal/index"
	"github.com/kokora3/zion/internal/protocol"
	"github.com/kokora3/zion/internal/research"
	"github.com/kokora3/zion/internal/resources"
)

type ObjectAvailability struct {
	ObjectID     string `json:"object_id"`
	PresentLocal bool   `json:"present_local"`
}

type ResearchView struct {
	index.ResearchEntry
	Objects []ObjectAvailability `json:"object_availability"`
}

type ResourceView struct {
	index.ResourceEntry
	Objects []ObjectAvailability `json:"object_availability"`
}

func (r *Runtime) ResearchList(offset, limit int) (any, error) {
	entries, err := r.registryIndex.ResearchList(offset, limit)
	if err != nil {
		return nil, err
	}
	result := make([]ResearchView, len(entries))
	for position, entry := range entries {
		result[position] = r.researchView(entry)
	}
	return result, nil
}

func (r *Runtime) Research(value string) (any, bool, error) {
	id, err := research.ParseID(value)
	if err != nil {
		return nil, false, err
	}
	entry, ok := r.registryIndex.Research(id.String())
	if !ok {
		return nil, false, nil
	}
	return r.researchView(entry), true, nil
}

func (r *Runtime) ResearchSearch(query string, offset, limit int) (any, error) {
	entries, err := r.registryIndex.ResearchSearch(query, offset, limit)
	if err != nil {
		return nil, err
	}
	result := make([]ResearchView, len(entries))
	for position, entry := range entries {
		result[position] = r.researchView(entry)
	}
	return result, nil
}

func (r *Runtime) ResourceList(offset, limit int) (any, error) {
	entries, err := r.registryIndex.ResourceList(offset, limit)
	if err != nil {
		return nil, err
	}
	result := make([]ResourceView, len(entries))
	for position, entry := range entries {
		result[position] = r.resourceView(entry)
	}
	return result, nil
}

func (r *Runtime) Resource(value string) (any, bool, error) {
	id, err := resources.ParseID(value)
	if err != nil {
		return nil, false, err
	}
	entry, ok := r.registryIndex.Resource(id.String())
	if !ok {
		return nil, false, nil
	}
	return r.resourceView(entry), true, nil
}

func (r *Runtime) ResourceSearch(query string, offset, limit int) (any, error) {
	entries, err := r.registryIndex.ResourceSearch(query, offset, limit)
	if err != nil {
		return nil, err
	}
	result := make([]ResourceView, len(entries))
	for position, entry := range entries {
		result[position] = r.resourceView(entry)
	}
	return result, nil
}

func (r *Runtime) researchView(entry index.ResearchEntry) ResearchView {
	return ResearchView{ResearchEntry: entry, Objects: r.objectAvailability(entry.Body.ObjectRefs)}
}

func (r *Runtime) resourceView(entry index.ResourceEntry) ResourceView {
	return ResourceView{ResourceEntry: entry, Objects: r.objectAvailability(entry.Body.ObjectRefs)}
}

func (r *Runtime) objectAvailability(ids []protocol.ObjectID) []ObjectAvailability {
	result := make([]ObjectAvailability, len(ids))
	for position, id := range ids {
		present, err := r.objects.Has(context.Background(), id)
		result[position] = ObjectAvailability{ObjectID: id.String(), PresentLocal: err == nil && present}
	}
	return result
}
