# Error Handling Guidelines

This project uses standard Go error chaining and `go.uber.org/multierr` for aggregation.

## Core Rules

1. Wrap boundary errors with context:
   - Use `fmt.Errorf("<operation>: %w", err)`.
   - Include actionable fields (path, ID, resource name) when safe.
2. Aggregate multiple independent errors with `multierr.Append`:
   - Keep each sub-error wrapped with operation context before appending.
   - Return a single wrapped aggregate at the boundary.
3. Preserve chain semantics:
   - Avoid string-only concatenation that loses causes.
   - Prefer `%w` so `errors.Is` / `errors.As` remain usable.
4. Keep user-facing text stable unless behavior changes.

## Example

```go
var agg error

if err := stopContainer(id); err != nil {
    agg = multierr.Append(agg, fmt.Errorf("stop container %s: %w", id, err))
}
if err := deleteNetwork(netID); err != nil {
    agg = multierr.Append(agg, fmt.Errorf("delete network %s: %w", netID, err))
}

if agg != nil {
    return fmt.Errorf("rollback failed for %s: %w", devnetName, agg)
}
```
