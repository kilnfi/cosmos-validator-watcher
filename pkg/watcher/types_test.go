package watcher

import (
	"testing"

	"gotest.tools/assert"
)

func TestTrackedValidator(t *testing.T) {

	t.Run("AccountAddress", func(t *testing.T) {
		testdata := []struct {
			Address string
			Account string
		}{
			{
				Address: "cosmosvaloper1uxlf7mvr8nep3gm7udf2u9remms2jyjqvwdul2",
				Account: "cosmos1uxlf7mvr8nep3gm7udf2u9remms2jyjqf6efne",
			},
			{
				Address: "cosmosvaloper1n229vhepft6wnkt5tjpwmxdmcnfz55jv3vp77d",
				Account: "cosmos1n229vhepft6wnkt5tjpwmxdmcnfz55jv5c4tj7",
			},
		}

		for _, td := range testdata {

			v := TrackedValidator{
				OperatorAddress: td.Address,
			}

			assert.Equal(t, v.AccountAddress(), td.Account)
		}
	})
}
