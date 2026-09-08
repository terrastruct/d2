package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
)

// These compact goldens cover complete placement, routing, labeling, and
// serialization results for representative algorithm domains at seeds beyond
// the main layout golden's seed. An intentional output change must review and
// update the corresponding digest.
func TestLayoutOutputAcrossSeeds(t *testing.T) {
	testCases := []struct {
		fixture string
		seed    int64
		sha256  string
	}{
		{fixture: "clusters", seed: 1, sha256: "79d04c0e5c4d00327a5d286e444446035a1c258981e6b07f161ad730c719abdd"},
		{fixture: "clusters", seed: 123, sha256: "a7c8a0c80712a10754d0925c84ccd6cd880bb7497fefca69c86a286fb19cd7d1"},
		{fixture: "clusters", seed: 8675309, sha256: "4f742ab0d3199a66b716e873fdba4c8bb9313d62c0a7a0102d710b7e82faf60f"},
		{fixture: "hierarchy_subgraphs", seed: 1, sha256: "507b98b3e70c735c17ce177400737f86b29210276cded1d7e7d21aed0ac0812c"},
		{fixture: "hierarchy_subgraphs", seed: 123, sha256: "99736f057c8fe8029c214df49a68355e0538641f841218caa0e52a0d8973764e"},
		{fixture: "hierarchy_subgraphs", seed: 8675309, sha256: "c3566b4aac683d5571016b8db25bac771dd1e038199d55cd6a70bf8f6747efff"},
		{fixture: "simple_container_hierarchy", seed: 1, sha256: "c0acc7c6967e39fe42d1caca89812e907e316e6067e601af1d9032a3d064714b"},
		{fixture: "simple_container_hierarchy", seed: 123, sha256: "41372d36020e12c44bdf9135fe42cb565dcce5a8cdb69c3ae3c4647a49574121"},
		{fixture: "simple_container_hierarchy", seed: 8675309, sha256: "d7e4ef56bef99c4ae854a5a44d45dcffd93513a09b1b88719a5c5cb261101485"},
		{fixture: "tree-n-seq", seed: 1, sha256: "2e91d8a8758012a6f88110f7033a21fb45a6543f02537757efde484735bfebe0"},
		{fixture: "tree-n-seq", seed: 123, sha256: "23d270f80f044eff9db4cfff2f52c2124b952b29177e3442665e1f9ed5cce14b"},
		{fixture: "tree-n-seq", seed: 8675309, sha256: "68f40e15b4794a5b6704dccf6fd93a9de446780bb2195ada80192919b6ddf0a9"},
	}

	for _, tc := range testCases {
		t.Run(tc.fixture+"/"+strconv.FormatInt(tc.seed, 10), func(t *testing.T) {
			sourcePath := filepath.Join(layoutTestDir, tc.fixture, "graph.input.json")
			input, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			temporaryPath := filepath.Join(t.TempDir(), "graph.input.json")
			if err := os.WriteFile(temporaryPath, input, 0o600); err != nil {
				t.Fatal(err)
			}

			ctx := withTestLogger(t.Context(), t)
			graph, err := readGraph(ctx, temporaryPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := layoutWithSnapshots(ctx, graph, tc.seed, false); err != nil {
				t.Fatal(err)
			}
			serialized, err := graphjson.Serialize(ctx, graph)
			if err != nil {
				t.Fatal(err)
			}
			output, err := json.Marshal(serialized)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(output)
			got := hex.EncodeToString(digest[:])
			if got != tc.sha256 {
				t.Fatalf("layout fingerprint = %s, want %s", got, tc.sha256)
			}
		})
	}
}
