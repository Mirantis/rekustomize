package runner

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/Mirantis/ktl/pkg/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const DefaultFileName = "rekustomization.yaml"

var (
	//go:embed defaults.yaml
	defaultsYaml []byte

	errUnsupportedKind = errors.New("unsupported source")
)

type Pipeline struct {
	Source Source `yaml:"source"`
	Output Output `yaml:"output"`

	Filters []filters.KFilter `yaml:"filters"`
}

type rekustomization Pipeline

func (cfg *Pipeline) UnmarshalYAML(node *yaml.Node) error {
	defaults := &rekustomization{}
	if err := yaml.Unmarshal(defaultsYaml, &defaults); err != nil {
		panic(fmt.Errorf("broken defaults: %w", err))
	}

	base := &rekustomization{}
	if err := node.Decode(base); err != nil {
		return fmt.Errorf("unable to parse config: %w", err)
	}

	cfg.Source = base.Source
	cfg.Output = base.Output
	cfg.Filters = base.Filters
	cfg.Filters = append(cfg.Filters, defaults.Filters...)

	return nil
}

func (cfg *Pipeline) Run(env *types.Env) error {
	filters := []kio.Filter{}

	for i := range cfg.Filters {
		filters = append(filters, cfg.Filters[i].Filter)
	}

	sres, err := cfg.Source.Load(env)
	if err != nil {
		return err //nolint:wrapcheck
	}

	ridx := map[resid.ResId]map[types.ClusterID]*yaml.RNode{}

	for clusterID, nodes := range sres.Resources {
		filtered := &kio.PackageBuffer{}
		pipeline := &kio.Pipeline{
			Inputs: []kio.Reader{
				&kio.PackageBuffer{
					Nodes: nodes,
				},
			},
			Outputs: []kio.Writer{filtered},
			Filters: filters,
		}

		if err := pipeline.Execute(); err != nil {
			return err //nolint:wrapcheck
		}

		for _, node := range filtered.Nodes {
			nodeID := resid.FromRNode(node)

			byCluster, idFound := ridx[nodeID]
			if !idFound {
				byCluster = map[types.ClusterID]*yaml.RNode{}
				ridx[nodeID] = byCluster
			}

			byCluster[clusterID] = node
		}
	}

	cres := &types.ClusterResources{
		Clusters:  sres.Clusters,
		Resources: ridx,
	}

	return cfg.Output.Store(env, cres) //nolint:wrapcheck
}
