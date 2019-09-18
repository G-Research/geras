#!/bin/bash
# Test flags in README are in sync with what the binary has.

if ! [[ -f README.md ]] || ! [[ -f geras ]]; then
  echo "Run in root of project with built binary too"
  exit 1
fi

readme="$(sed -n '/^## Usage/,/^#/p' README.md)"
usage="$(./geras -help 2>&1)"

readme_flags="$(egrep '^\s*-' <<< "$readme" | sort | awk '{print $1}')"
usage_flags="$(egrep '^\s*-' <<< "$usage" | sort | awk '{print $1}')"

echo  "Testing flags ./geras vs README.md:"
# Exit code of diff is the result of the test
diff -u <(cat <<<"$usage_flags") <(cat <<<"$readme_flags") && echo "OK"
