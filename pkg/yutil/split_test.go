package yutil_test

import (
	"strings"
	"testing"

	"github.com/Mirantis/rekustomize/pkg/yutil"
	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func testSplit(t *testing.T, rn *yaml.RNode, wantPaths []yutil.Path, wantValues []string) {
	gotPaths := []yutil.Path{}
	gotValues := []string{}
	entries, err := yutil.Split(rn)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		gotPaths = append(gotPaths, e.Path)
		text, err := e.String()
		if err != nil {
			t.Error(err)
		}
		gotValues = append(gotValues, strings.TrimRight(text, "\n"))
	}

	if diff := cmp.Diff(gotPaths, wantPaths); diff != "" {
		t.Errorf("unexpected paths, +got -want:\n %v", diff)
	}

	if diff := cmp.Diff(gotValues, wantValues); diff != "" {
		t.Errorf("unexpected values, +got -want:\n %v", diff)
	}
}

func TestSplitScalar(t *testing.T) {
	tests := map[string]struct {
		rn         *yaml.RNode
		wantPaths  []yutil.Path
		wantValues []string
	}{
		"nil": {
			rn:         nil,
			wantPaths:  []yutil.Path{nil},
			wantValues: []string{""},
		},
		"empty string": {
			rn:         yaml.NewStringRNode(""),
			wantPaths:  []yutil.Path{nil},
			wantValues: []string{`""`},
		},
		"string": {
			rn:         yaml.NewStringRNode("abc"),
			wantPaths:  []yutil.Path{nil},
			wantValues: []string{`abc`},
		},
		"number": {
			rn:         yaml.NewScalarRNode("123"),
			wantPaths:  []yutil.Path{nil},
			wantValues: []string{`123`},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			testSplit(t, test.rn, test.wantPaths, test.wantValues)
		})
	}
}
