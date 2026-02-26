#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

upgrade_file="cmd/devnet-builder/commands/manage/upgrade.go"
deploy_file="cmd/devnet-builder/commands/manage/deploy.go"
cc_limit=20
loc_limit=149

if ! command -v gocyclo >/dev/null 2>&1; then
  echo "gocyclo is required but not found in PATH" >&2
  exit 1
fi

cc_output="$(gocyclo "$upgrade_file" "$deploy_file")"

extract_cc() {
  local fn="$1"
  echo "$cc_output" | awk -v fn="$fn" '$3==fn {print $1; exit}'
}

cc_run_upgrade="$(extract_cc runUpgrade)"
cc_run_deploy="$(extract_cc runDeploy)"

if [[ -z "$cc_run_upgrade" || -z "$cc_run_deploy" ]]; then
  echo "failed to locate runUpgrade/runDeploy in gocyclo output" >&2
  echo "$cc_output" >&2
  exit 1
fi

tmp_base="$(mktemp /tmp/manage-func-loc-XXXXXX)"
tmp_go="${tmp_base}.go"
mv "$tmp_base" "$tmp_go"
cat > "$tmp_go" <<'GOEOF'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: loc <file>...")
		os.Exit(1)
	}

	var paths []string
	for _, arg := range os.Args[1:] {
		if arg == "--" {
			continue
		}
		paths = append(paths, arg)
	}

	if len(paths) == 0 {
		fmt.Println("no files provided")
		os.Exit(1)
	}

	fset := token.NewFileSet()
	loc := map[string]int{}

	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse error %s: %v\n", path, err)
			os.Exit(1)
		}
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if fd.Name.Name != "runUpgrade" && fd.Name.Name != "runDeploy" {
				continue
			}
			start := fset.Position(fd.Pos()).Line
			end := fset.Position(fd.End()).Line
			loc[fd.Name.Name] = end - start + 1
		}
	}

	fmt.Printf("runUpgrade=%d\n", loc["runUpgrade"])
	fmt.Printf("runDeploy=%d\n", loc["runDeploy"])
}
GOEOF

loc_output="$(go run "$tmp_go" -- "$upgrade_file" "$deploy_file")"
rm -f "$tmp_go"

extract_loc() {
  local fn="$1"
  echo "$loc_output" | awk -F= -v fn="$fn" '$1==fn {print $2; exit}'
}

loc_run_upgrade="$(extract_loc runUpgrade)"
loc_run_deploy="$(extract_loc runDeploy)"

if [[ -z "$loc_run_upgrade" || -z "$loc_run_deploy" ]]; then
  echo "failed to locate runUpgrade/runDeploy LOC output" >&2
  echo "$loc_output" >&2
  exit 1
fi

printf 'runUpgrade: CC=%s LOC=%s\n' "$cc_run_upgrade" "$loc_run_upgrade"
printf 'runDeploy:  CC=%s LOC=%s\n' "$cc_run_deploy" "$loc_run_deploy"

if (( cc_run_upgrade > cc_limit )); then
  echo "runUpgrade CC ${cc_run_upgrade} exceeds ${cc_limit}" >&2
  exit 1
fi
if (( cc_run_deploy > cc_limit )); then
  echo "runDeploy CC ${cc_run_deploy} exceeds ${cc_limit}" >&2
  exit 1
fi
if (( loc_run_upgrade > loc_limit )); then
  echo "runUpgrade LOC ${loc_run_upgrade} exceeds ${loc_limit}" >&2
  exit 1
fi
if (( loc_run_deploy > loc_limit )); then
  echo "runDeploy LOC ${loc_run_deploy} exceeds ${loc_limit}" >&2
  exit 1
fi
