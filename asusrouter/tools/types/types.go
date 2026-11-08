// Package types ports `asusrouter/tools/types`: the universal callback
// types.
package types

import "context"

// ARCallableType mirrors `ARCallableType`: universal sync callback.
type ARCallableType = func(args ...any) any

// ARCallbackType mirrors `ARCallbackType`: universal async callback
// (Go: a context-aware call returning an error instead of raising).
type ARCallbackType = func(ctx context.Context, args ...any) (any, error)
