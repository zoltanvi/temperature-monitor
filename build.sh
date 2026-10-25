#!/bin/bash

echo "Select target:"
echo "1) windows/amd64"
echo "2) linux/arm64"
read -p "Choice [1-2]: " target

case $target in
  1)
    GOOS=windows
    GOARCH=amd64
    DIR=windows-amd64
    EXT=".exe"
    ;;
  2)
    GOOS=linux
    GOARCH=arm64
    DIR=linux-arm64
    EXT=""
    ;;
  *)
    echo "Invalid choice"
    exit 1
    ;;
esac

OUT_DIR="build/$DIR"
mkdir -p "$OUT_DIR"

for PKG in ./cmd/api ./cmd/sensor; do
    BINARY_NAME=${PKG##*/}
    OUT_FILE="$OUT_DIR/$BINARY_NAME$EXT"

    echo "Building $PKG -> $OUT_FILE ($GOOS/$GOARCH)"
    GOOS=$GOOS GOARCH=$GOARCH go build -o "$OUT_FILE" "$PKG" || exit 1

    if [ "$GOOS" = "linux" ]; then
        chmod +x "$OUT_FILE"
    fi
done

echo "Done."
