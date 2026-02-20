package stateexport

import (
	"reflect"
	"testing"
)

func TestRewriteHomeDirArgsForDocker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		hostHome      string
		containerHome string
		want          []string
	}{
		{
			name:          "rewrites --home value",
			args:          []string{"export", "--home", "/tmp/state-export"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home", "/data"},
		},
		{
			name:          "rewrites --home equals syntax",
			args:          []string{"export", "--home=/tmp/state-export"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home=/data"},
		},
		{
			name:          "rewrites nested home path",
			args:          []string{"export", "--home", "/tmp/state-export/node0"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home", "/data/node0"},
		},
		{
			name:          "leaves unrelated absolute path untouched",
			args:          []string{"export", "--home", "/opt/other-home"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home", "/opt/other-home"},
		},
		{
			name:          "leaves relative home untouched",
			args:          []string{"export", "--home", "relative-home"},
			hostHome:      "/tmp/state-export",
			containerHome: "/data",
			want:          []string{"export", "--home", "relative-home"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := append([]string(nil), tc.args...)
			got := rewriteHomeDirArgsForDocker(tc.args, tc.hostHome, tc.containerHome)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rewriteHomeDirArgsForDocker() = %#v, want %#v", got, tc.want)
			}

			if !reflect.DeepEqual(tc.args, original) {
				t.Fatalf("rewriteHomeDirArgsForDocker mutated input args")
			}
		})
	}
}
