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

        echo "Original file size of $OUTPUT_FILE: $FILE_SIZE_MB MB"

        # Try to optimize with wasm-opt if available
        if command -v wasm-opt >/dev/null 2>&1; then
            echo "Optimizing with wasm-opt (this may take a moment)..."
            if wasm-opt -Oz --enable-bulk-memory-opt "$OUTPUT_FILE" -o "$OUTPUT_FILE" 2>/dev/null; then
                # Measure optimized size
                OPTIMIZED_SIZE_BYTES=$(stat -f%z "$OUTPUT_FILE")
                OPTIMIZED_SIZE_MB=$(echo "scale=2; $OPTIMIZED_SIZE_BYTES / 1024 / 1024" | bc)

                echo "Optimized file size of $OUTPUT_FILE: $OPTIMIZED_SIZE_MB MB"

                # Calculate reduction
                REDUCTION=$(echo "scale=2; $FILE_SIZE_MB - $OPTIMIZED_SIZE_MB" | bc)
                echo "Size reduction: $REDUCTION MB"
            else
                echo "wasm-opt optimization timed out or failed, showing original size only"
            fi
        else
            echo "wasm-opt not found, skipping optimization"
            echo "To install: brew install binaryen"
        fi
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
