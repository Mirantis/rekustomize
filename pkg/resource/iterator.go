package resource

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/Mirantis/ktl/pkg/types"
	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/kyaml/yaml/schema"
)

type Iterator struct {
	clusters []types.ClusterID
	states   stack[*iteratorState]
	current  *iteratorState
	err      error
}

func NewIterator(resources map[types.ClusterID]*yaml.RNode, schema *openapi.ResourceSchema) *Iterator {
	resIter := &Iterator{}
	if len(resources) == 0 {
		return resIter
	}

	resIter.clusters = slices.Sorted(maps.Keys(resources))
	state := &iteratorState{
		schema:  schema,
		values:  make([]*yaml.Node, len(resIter.clusters)),
		indices: make([]int, len(resIter.clusters)),
	}

	for i, cluster := range resIter.clusters {
		state.values[i] = resources[cluster].YNode()
	}

	resIter.states.push(state)

	return resIter
}

func (it *Iterator) Error() error {
	return it.err
}

func (it *Iterator) Path() Query {
	return it.current.path
}

func (it *Iterator) Schema() *openapi.ResourceSchema {
	return it.current.schema
}

func (it *Iterator) Clusters() []types.ClusterID {
	clusters := make([]types.ClusterID, 0, len(it.clusters))
	for cluster := range it.Values() {
		clusters = append(clusters, cluster)
	}

	return clusters
}

func (it *Iterator) Values() iter.Seq2[types.ClusterID, *yaml.Node] {
	return func(yield func(types.ClusterID, *yaml.Node) bool) {
		placeholder := &yaml.Node{Kind: it.current.kind}

		for idx := range it.clusters {
			value := it.current.values[idx]
			if value == nil {
				continue
			}

			if !it.current.isValue {
				value = placeholder
			}

			if !yield(it.clusters[idx], value) {
				return
			}
		}
	}
}

func (it *Iterator) Next() bool {
	if len(it.states) == 0 {
		return false
	}

	it.current = it.states.pop()

	batch, err := it.current.unfold()
	if err != nil {
		it.err = err

		return false
	}

	sort.Sort(iteratorStatesOrder(batch))
	it.states.push(batch...)

	return true
}

type iteratorState struct {
	schema  *openapi.ResourceSchema
	path    Query
	values  []*yaml.Node
	indices []int
	kind    yaml.Kind
	isValue bool
}

var errNodeKind = errors.New("invalid node kind")

func (is *iteratorState) init() error {
	if is.kind != 0 {
		return nil
	}

	for _, node := range is.values {
		if nil == node || is.kind == node.Kind {
			continue
		}

		if is.kind != 0 {
			return fmt.Errorf("%w: %s", errNodeKind, is.path)
		}

		is.kind = node.Kind
	}

	switch is.kind {
	case yaml.ScalarNode:
		is.isValue = true
	case yaml.MappingNode:
		is.isValue = false
	case yaml.SequenceNode:
		is.isValue = !schema.IsAssociative(is.schema, nil, false)
	default:
		return fmt.Errorf("%w: %s", errNodeKind, is.path)
	}

	return nil
}

func (is *iteratorState) mergeKey() []string {
	_, key := is.schema.PatchStrategyAndKeyList()

	return key
}

func (is *iteratorState) unfold() ([]*iteratorState, error) {
	if err := is.init(); err != nil {
		return nil, err
	}

	if is.isValue {
		return nil, nil
	}

	switch is.kind {
	case yaml.MappingNode:
		return is.mappingFields()
	case yaml.SequenceNode:
		return is.listElements()
	default:
		panic(fmt.Errorf("%w %v: %s", errNodeKind, is.kind, is.path))
	}
}

func (is *iteratorState) mappingFields() ([]*iteratorState, error) {
	states := map[string]*iteratorState{}

	for idxValue, node := range is.values {
		if node == nil {
			continue
		}

		if len(node.Content)%2 != 0 {
			panic(fmt.Errorf("%w: %s", errNodeKind, is.path))
		}

		for idxField := range len(node.Content) / 2 {
			pathPart := node.Content[idxField*2].Value
			state, exists := states[pathPart]

			if !exists {
				state = &iteratorState{
					path:    append(slices.Clone(is.path), pathPart),
					values:  make([]*yaml.Node, len(is.values)),
					indices: make([]int, len(is.indices)),
				}
				states[pathPart] = state

				if is.schema != nil {
					state.schema = is.schema.Field(pathPart)
				}
			}

			state.indices[idxValue] = idxField
			state.values[idxValue] = node.Content[idxField*2+1]
		}
	}

	return slices.Collect(maps.Values(states)), nil
}

var errInvalidKV = errors.New("invalid keys/values")

func kvPathPart(key, values []string) string {
	if len(key) != len(values) {
		panic(errInvalidKV)
	}

	parts := make([]string, len(key))
	for i := range key {
		parts[i] = key[i] + "=" + values[i]
	}

	return "[" + strings.Join(parts, ",") + "]"
}

func (is *iteratorState) listElements() ([]*iteratorState, error) {
	schema := is.schema.Elements()
	key := is.mergeKey()
	states := map[string]*iteratorState{}
	allKeyValues := make([][][]string, len(is.values))

	for idx, node := range is.values {
		if node == nil {
			continue
		}

		resNode := yaml.NewRNode(node)

		evl, err := resNode.ElementValuesList(key)
		if err != nil {
			return nil, fmt.Errorf("invalid yaml: %w", err)
		}

		allKeyValues[idx] = evl
	}

	for idxValue, keyValues := range allKeyValues {
		for idxElement, elementValues := range keyValues {
			pathPart := kvPathPart(key, elementValues)
			state, exists := states[pathPart]

			if !exists {
				state = &iteratorState{
					schema:  schema,
					path:    append(slices.Clone(is.path), pathPart),
					values:  make([]*yaml.Node, len(is.values)),
					indices: make([]int, len(is.indices)),
				}
				states[pathPart] = state
			}

			state.indices[idxValue] = idxElement
			state.values[idxValue] = is.values[idxValue].Content[idxElement]
		}
	}

	return slices.Collect(maps.Values(states)), nil
}

type iteratorStatesOrder []*iteratorState

func (o iteratorStatesOrder) Len() int      { return len(o) }
func (o iteratorStatesOrder) Swap(a, b int) { o[a], o[b] = o[b], o[a] }
func (o iteratorStatesOrder) Less(a, b int) bool { //nolint:varnamelen
	if d := o.byIndices(a, b); d != 0 {
		return d < 0
	}

	if d := o.byNils(a, b); d != 0 {
		return d < 0
	}

	return o.byPath(a, b) < 0
}

func (o iteratorStatesOrder) byIndices(a, b int) int {
	isa, isb := o[a], o[b]
	// REVISIT: replace sum with a more reliable condition
	suma, sumb := 0, 0
	for i := range max(len(isa.indices), len(isb.indices)) {
		suma += isa.indices[i]
		sumb += isb.indices[i]
	}

	return suma - sumb
}

func (o iteratorStatesOrder) byNils(a, b int) int {
	isa, isb := o[a], o[b]
	suma, sumb := 0, 0

	for i := range max(len(isa.indices), len(isb.indices)) {
		if isa.values[i] == nil {
			suma--
		}

		if isb.values[i] == nil {
			sumb--
		}
	}

	return suma - sumb
}

func (o iteratorStatesOrder) byPath(a, b int) int {
	isa, isb := o[a], o[b]
	pa := isa.path[len(isa.path)-1]
	pb := isb.path[len(isb.path)-1]

	return strings.Compare(pa, pb)
}
