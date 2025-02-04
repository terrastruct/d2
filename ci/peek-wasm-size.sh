#!/bin/bash

OUTPUT_FILE="main.wasm"
SOURCE_PACKAGE="./d2js"

echo "Building WASM file..."
GOOS=js GOARCH=wasm go build -ldflags='-s -w' -trimpath -o "$OUTPUT_FILE" "$SOURCE_PACKAGE"

if [ $? -eq 0 ]; then
    echo "Build successful."

    if [ -f "$OUTPUT_FILE" ]; then
        FILE_SIZE_BYTES=$(stat -f%z "$OUTPUT_FILE")
        FILE_SIZE_MB=$(echo "scale=2; $FILE_SIZE_BYTES / 1024 / 1024" | bc)

        echo "File size of $OUTPUT_FILE: $FILE_SIZE_MB MB"
    else
        echo "File $OUTPUT_FILE not found!"
        exit 1
    fi

    echo "Deleting $OUTPUT_FILE..."
    rm "$OUTPUT_FILE"
    echo "File deleted."
else
    echo "Build failed."
    exit 1
fi
