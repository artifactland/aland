package cli

import "context"

type ctxKey int

const globalsKey ctxKey = 1

// WithGlobals stashes the global-flags struct in a context. Subcommands
// retrieve it via Globals(ctx).
func WithGlobals(ctx context.Context, g *GlobalFlags) context.Context {
	return context.WithValue(ctx, globalsKey, g)
}

// Globals reads the global flags off the context, returning an empty value
// when the context is nil or nothing was stashed (e.g., tests that build a
// cobra.Command directly).
func Globals(ctx context.Context) *GlobalFlags {
	if ctx == nil {
		return &GlobalFlags{}
	}
	if g, ok := ctx.Value(globalsKey).(*GlobalFlags); ok && g != nil {
		return g
	}
	return &GlobalFlags{}
}
