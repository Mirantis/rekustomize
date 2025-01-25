package types_test

import (
	"testing"

	"github.com/Mirantis/rekustomize/pkg/types"
)

func TestClusterIndexGroup(t *testing.T) {
	idx := types.NewClusterIndex()
	c1 := idx.Add(types.Cluster{Name: "c1", Tags: []string{"a", "b", "c"}})
	c2 := idx.Add(types.Cluster{Name: "c2", Tags: []string{"a"}})
	c3 := idx.Add(types.Cluster{Name: "c3", Tags: []string{"b"}})
	c4 := idx.Add(types.Cluster{Name: "c4", Tags: []string{"c"}})
	c5 := idx.Add(types.Cluster{Name: "c5", Tags: []string{}})

	tests := []struct {
		want string
		ids  []types.ClusterId
	}{
		{
			want: "all-clusters",
			ids:  []types.ClusterId{c1, c2, c3, c4, c5},
		},
		{
			want: "a+b+c",
			ids:  []types.ClusterId{c1, c2, c3, c4},
		},
		{
			want: "a+c",
			ids:  []types.ClusterId{c1, c2, c4},
		},
		{
			want: "a",
			ids:  []types.ClusterId{c1, c2},
		},
		{
			want: "c",
			ids:  []types.ClusterId{c1, c4},
		},
		{
			want: "c2+c3+c4+c5",
			ids:  []types.ClusterId{c2, c3, c4, c5},
		},
		{
			want: "c1+c5",
			ids:  []types.ClusterId{c1, c5},
		},
	}

	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			got := idx.Group(test.ids...)
			if got == test.want {
				return
			}
			t.Errorf("got: %s, want: %s", got, test.want)
		})
	}
}
