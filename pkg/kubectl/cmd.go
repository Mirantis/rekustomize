package kubectl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"k8s.io/kubectl/pkg/cmd/version"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func DefaultCmd() Cmd {
	return []string{"kubectl"}
}

type Cmd []string

func (kc Cmd) String() string {
	return strings.Join(kc, " ")
}

func (kc Cmd) Server(server string) Cmd {
	return slices.Concat(kc, []string{"--server", server})
}

func (kc Cmd) Cluster(cluster string) Cmd {
	return slices.Concat(kc, []string{"--cluster", cluster})
}

func (kc Cmd) output(args ...string) ([]byte, error) {
	cmd := exec.Command(kc[0], slices.Concat(kc[1:], args)...)
	data, err := cmd.Output()

	switch err := err.(type) {
	case nil:
		return data, nil
	case *exec.ExitError:
		stderr := strings.TrimSpace(string(err.Stderr))
		return nil, fmt.Errorf("%s failed: %v, stderr: %s", kc, err, stderr)
	default:
		return nil, err
	}
}

func (kc Cmd) Version(server bool) (*version.Version, error) {
	args := []string{"version", "-ojson"}
	if !server {
		args = append(args, "--client=true")
	}

	data, err := kc.output(args...)
	if err != nil {
		return nil, err
	}

	v := &version.Version{}
	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}

	return v, nil
}

func (kc Cmd) ApplyKustomization(path string) error {
	_, err := kc.output("apply", "--kustomize", path)
	return err
}

func (kc Cmd) Get(resources, namespace string, selectors []string, names ...string) ([]*yaml.RNode, error) {
	args := []string{"get", "-oyaml", resources}
	if len(selectors) > 0 {
		args = append(args, "-l", strings.Join(selectors, ","))
	}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	response, err := kc.output(args...)
	if err != nil {
		return nil, err
	}
	root, err := yaml.Parse(string(response))
	if err != nil {
		return nil, err
	}
	items, err := root.Pipe(yaml.Lookup("items"))
	if err != nil {
		return nil, err
	}
	nodes := []*yaml.RNode{}
	for _, item := range items.Content() {
		rn := yaml.NewRNode(item)
		if len(names) > 0 && !slices.Contains(names, rn.GetName()) {
			continue
		}
		nodes = append(nodes, rn)
	}
	return nodes, nil
}

func (kc Cmd) GetAll(namespace string, selectors []string, kinds ...string) ([]*yaml.RNode, error) {
	// REVISIT: extract common code for Get/GetAll
	if len(kinds) < 1 {
		kinds = []string{"all"}
	}
	args := []string{"get", "-oyaml", strings.Join(kinds, ",")}
	if len(selectors) > 0 {
		args = append(args, "-l", strings.Join(selectors, ","))
	}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	response, err := kc.output(args...)
	if err != nil {
		return nil, err
	}
	root, err := yaml.Parse(string(response))
	if err != nil {
		return nil, err
	}
	items, err := root.Pipe(yaml.Lookup("items"))
	if err != nil {
		return nil, err
	}
	nodes := []*yaml.RNode{}
	for _, item := range items.Content() {
		rn := yaml.NewRNode(item)
		nodes = append(nodes, rn)
	}
	return nodes, nil
}

func (kc Cmd) ApiResources(namespaced bool) ([]string, error) {
	response, err := kc.output(
		"api-resources",
		"-o", "name",
		"--verbs", "get",
		"--namespaced="+fmt.Sprint(namespaced),
	)
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(bytes.NewBuffer(response))
	resources := []string{}
	for s.Scan() {
		resources = append(resources, s.Text())
	}
	return resources, nil
}

func (kc Cmd) Namespaces() ([]string, error) {
	response, err := kc.output(
		"get", "namespaces",
		"-o", "name",
		"--no-headers",
	)
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(bytes.NewBuffer(response))
	namespaces := []string{}
	for s.Scan() {
		namespace, found := strings.CutPrefix(s.Text(), "namespace/")
		if !found {
			continue
		}
		namespaces = append(namespaces, namespace)
	}
	slices.Sort(namespaces)
	return namespaces, nil
}

func (kc Cmd) Clusters() ([]string, error) {
	response, err := kc.output("config", "get-clusters")
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(bytes.NewBuffer(response))
	clusters := []string{}
	for s.Scan() {
		cluster := s.Text()
		if "NAME" == cluster {
			continue
		}
		clusters = append(clusters, cluster)
	}
	slices.Sort(clusters)
	return clusters, nil
}
