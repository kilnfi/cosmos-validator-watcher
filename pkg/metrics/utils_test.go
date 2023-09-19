package metrics

import (
	"testing"

	"gotest.tools/assert"
)

func TestBoolToFloat64(t *testing.T) {
	assert.Equal(t, float64(1), BoolToFloat64(true))
	assert.Equal(t, float64(0), BoolToFloat64(false))
}
