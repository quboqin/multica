package capability

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

const CreativeFactory = "creative_factory"

type Definition struct {
	Key           string
	DefaultEnable bool
}

var definitions = []Definition{
	{Key: CreativeFactory, DefaultEnable: false},
}

func Definitions() []Definition {
	return append([]Definition(nil), definitions...)
}

func Lookup(key string) (Definition, bool) {
	for _, definition := range definitions {
		if definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}

type Reader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Enabled(ctx context.Context, reader Reader, workspaceID, key string) (bool, error) {
	if _, ok := Lookup(key); !ok {
		return false, nil
	}
	var enabled bool
	err := reader.QueryRow(ctx, `
SELECT enabled
FROM workspace_capability
WHERE workspace_id = $1::uuid AND capability_key = $2
`, workspaceID, key).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}
