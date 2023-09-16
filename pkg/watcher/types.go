package watcher

type NodeEvent[T any] struct {
	Endpoint string
	Data     T
}
