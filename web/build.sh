#!/bin/sh

BASE_DIR=$(cd "$(dirname "$0")" && pwd)
version=$(cat "$BASE_DIR/VERSION")
pwd

while IFS= read -r theme; do
    echo "Building theme: $theme"
    rm -r "$BASE_DIR/build/$theme"
    cd "$BASE_DIR/$theme"
    npm install --legacy-peer-deps
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$version npm run build
    cd "$BASE_DIR"
done < "$BASE_DIR/THEMES"
