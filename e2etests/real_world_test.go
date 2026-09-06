package e2etests

import "testing"

// Sources and licenses are recorded in testdata/files/REAL_WORLD.md.
func testRealWorld(t *testing.T) {
	runa(t, []testCase{
		loadFromFile(t, "jupyter_aws_eks"),
		loadFromFile(t, "ross_overview"),
		loadFromFile(t, "spyre_encoder"),
		loadFromFile(t, "queue_workers"),
		loadFromFile(t, "mocha_soc"),
		loadFromFile(t, "tpmjs_architecture"),
		loadFromFile(t, "jupyter_k8s_oidc"),
		loadFromFile(t, "leios_simulator"),
		loadFromFile(t, "fulcro_rad"),
		loadFromFile(t, "lion_reader_frontend"),
	})
}
