package watcher

import (
	"errors"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/query"
	"gotest.tools/assert"
)

func TestPaginate(t *testing.T) {
	t.Run("follows next key until exhausted", func(t *testing.T) {
		pages := map[string]struct {
			items []int
			next  []byte
		}{
			"":  {items: []int{1, 2}, next: []byte("a")},
			"a": {items: []int{3, 4}, next: []byte("b")},
			"b": {items: []int{5}, next: nil},
		}

		var keys []string
		items, err := paginate(func(key []byte) ([]int, *query.PageResponse, error) {
			keys = append(keys, string(key))
			page := pages[string(key)]
			return page.items, &query.PageResponse{NextKey: page.next}, nil
		})

		assert.NilError(t, err)
		assert.DeepEqual(t, items, []int{1, 2, 3, 4, 5})
		assert.DeepEqual(t, keys, []string{"", "a", "b"})
	})

	t.Run("stops on nil page response", func(t *testing.T) {
		items, err := paginate(func(key []byte) ([]int, *query.PageResponse, error) {
			return []int{1}, nil, nil
		})

		assert.NilError(t, err)
		assert.DeepEqual(t, items, []int{1})
	})

	t.Run("returns error", func(t *testing.T) {
		_, err := paginate(func(key []byte) ([]int, *query.PageResponse, error) {
			return nil, nil, errors.New("limit exceeded")
		})

		assert.Error(t, err, "limit exceeded")
	})
}
