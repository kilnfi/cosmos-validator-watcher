package watcher

import "strings"

type TrackedValidator struct {
	Address         string
	Name            string
	Moniker         string
	OperatorAddress string
}

func ParseValidator(val string) TrackedValidator {
	parts := strings.Split(val, ":")
	if len(parts) > 1 {
		return TrackedValidator{
			Address: parts[0],
			Name:    parts[1],
		}
	}

	return TrackedValidator{
		Address: parts[0],
		Name:    parts[0],
	}
}
