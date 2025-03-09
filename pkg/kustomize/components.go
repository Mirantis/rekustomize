package kustomize

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Mirantis/rekustomize/pkg/resource"
	"github.com/Mirantis/rekustomize/pkg/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type component struct {
	name      string
	resources map[resid.ResId]*yaml.RNode
	patches   map[resid.ResId]*yaml.RNode
	clusters  []types.ClusterID
}

func (comp *component) filePath(resID resid.ResId) string {
	parts := []string{}
	if resID.Namespace != "" {
		parts = append(parts, resID.Namespace)
	}

	parts = append(parts, strings.ToLower(fmt.Sprintf("%s-%s.yaml", resID.Name, resID.Kind)))

	return filepath.Join(parts...)
}

func (comp *component) store(fileSys filesys.FileSystem, dir string) error {
	kust := &types.Kustomization{}
	kust.Kind = types.ComponentKind
	resourceStore := &resource.FileStore{
		Dir:           dir,
		FileSystem:    fileSys,
		NameGenerator: comp.filePath,
		PostProcessor: func(path string, body []byte) []byte {
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				panic(err)
			}
			kust.Resources = append(kust.Resources, relPath)

			return body
		},
	}

	if err := resourceStore.WriteAll(maps.All(comp.resources)); err != nil {
		return fmt.Errorf("unable to store component files: %w", err)
	}

	patches := []string{}
	patchStore := &resource.FileStore{
		Dir:           dir,
		FileSystem:    fileSys,
		NameGenerator: comp.filePath,
		PostProcessor: func(path string, body []byte) []byte {
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				panic(err)
			}
			patches = append(patches, relPath)

			return body
		},
	}

	if err := patchStore.WriteAll(maps.All(comp.patches)); err != nil {
		return fmt.Errorf("unable to store component files: %w", err)
	}

	slices.Sort(kust.Resources)
	slices.Sort(patches)

	for _, patch := range patches {
		kust.Patches = append(kust.Patches, types.Patch{Path: patch})
	}

	kustBody, err := yaml.Marshal(kust)
	if err != nil {
		return fmt.Errorf("unable to generate kustomization.yaml: %w", err)
	}

	err = fileSys.WriteFile(filepath.Join(dir, "kustomization.yaml"), kustBody)
	if err != nil {
		return fmt.Errorf("unable to store kustomization.yaml: %w", err)
	}

	return nil
}

type Components struct {
	clusters  *types.ClusterIndex
	byName    map[string]*component
	byCluster map[types.ClusterID][]*component
}

func NewComponents(clusters *types.ClusterIndex) *Components {
	comps := &Components{
		clusters:  clusters,
		byName:    map[string]*component{},
		byCluster: map[types.ClusterID][]*component{},
	}

	return comps
}

var errClusterNotFound = errors.New("cluster not found")

func (comps *Components) Cluster(cluster types.ClusterID) ([]string, error) {
	names := []string{}

	items, found := comps.byCluster[cluster]
	if !found {
		return nil, errClusterNotFound
	}

	sort.Sort(componentsOrder(items))

	for _, item := range items {
		names = append(names, item.name)
	}

	return names, nil
}

func (comps *Components) component(ids ...types.ClusterID) *component {
	name := comps.clusters.Group(ids...)

	comp, found := comps.byName[name]
	if !found {
		comp = &component{
			name:      name,
			clusters:  ids,
			resources: map[resid.ResId]*yaml.RNode{},
			patches:   map[resid.ResId]*yaml.RNode{},
		}
		comps.byName[name] = comp

		for _, id := range ids {
			comps.byCluster[id] = append(comps.byCluster[id], comp)
		}
	}

	return comp
}

func (comps *Components) Add(resID resid.ResId, resources map[types.ClusterID]*yaml.RNode) error {
	mainBuilder := resource.NewBuilder(resID)
	mainClusterIDs := slices.Collect(maps.Keys(resources))
	mainComp := comps.component(mainClusterIDs...)
	mainComp.resources[resID] = mainBuilder.RNode()
	builders := map[string]*resource.Builder{}
	schema := openapi.SchemaForResourceType(resID.AsTypeMeta())

	resIter := resource.NewIterator(resources, schema)
	for resIter.Next() {
		variants := resource.GroupByValue(resIter.Values())
		for _, variant := range variants {
			comp := mainComp
			builder := mainBuilder

			if len(variant.Clusters) != len(mainClusterIDs) {
				comp = comps.component(variant.Clusters...)
				builder = builders[comp.name]
			}

			if builder == nil {
				builder = resource.NewBuilder(resID)
				comp.patches[resID] = builder.RNode()
				builders[comp.name] = builder
			}

			if _, err := builder.Set(resIter.Path(), variant.Value); err != nil {
				return fmt.Errorf("unable to set %s for %s: %w", resIter.Path(), resID, err)
			}
		}
	}

	if err := resIter.Error(); err != nil {
		return fmt.Errorf("error while iterating over %s: %w", resID, err)
	}

	return nil
}

func (comps *Components) Store(fileSys filesys.FileSystem, dir string) error {
	for name, comp := range comps.byName {
		if err := comp.store(fileSys, filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("unable to store component %s: %w", name, err)
		}
	}

	return nil
}

type componentsOrder []*component

func (o componentsOrder) Len() int      { return len(o) }
func (o componentsOrder) Swap(a, b int) { o[a], o[b] = o[b], o[a] }
func (o componentsOrder) Less(a, b int) bool {
	if d := len(o[a].clusters) - len(o[b].clusters); d != 0 {
		return d > 0 // descending order
	}

	return strings.Compare(o[a].name, o[b].name) < 0
}
