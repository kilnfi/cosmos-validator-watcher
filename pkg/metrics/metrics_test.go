package metrics

import "testing"

func TestMetrics(t *testing.T) {
	m := New("cosmos_validator_watcher")
	m.Register()
}
