package png

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type timeoutRecorder struct {
	defaultCalled     bool
	navigationCalled  bool
	defaultTimeout    float64
	navigationTimeout float64
}

func (r *timeoutRecorder) SetDefaultTimeout(timeout float64) {
	r.defaultCalled = true
	r.defaultTimeout = timeout

}

func (r *timeoutRecorder) SetDefaultNavigationTimeout(timeout float64) {
	r.navigationCalled = true
	r.navigationTimeout = timeout
}

func TestConfigureTimeout(t *testing.T) {
	for _, tc := range []struct {
		name        string
		value       string
		wantCalled  bool
		wantTimeout float64
	}{
		{name: "unset"},
		{name: "seconds", value: "125", wantCalled: true, wantTimeout: 125000},
		{name: "disabled", value: "0", wantCalled: true},
		{name: "negative disables", value: "-1", wantCalled: true},
		{name: "invalid", value: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("D2_TIMEOUT", tc.value)
			recorder := &timeoutRecorder{}

			configureTimeout(recorder)

			assert.Equal(t, tc.wantCalled, recorder.defaultCalled)
			assert.Equal(t, tc.wantCalled, recorder.navigationCalled)
			assert.Equal(t, tc.wantTimeout, recorder.defaultTimeout)
			assert.Equal(t, tc.wantTimeout, recorder.navigationTimeout)
		})
	}
}
