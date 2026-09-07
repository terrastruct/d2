package routing

import "context"

// cachedContextErr avoids repeatedly traversing context value wrappers in hot
// guarded loops. Standard cancellable contexts close Done before Err becomes
// observable; synthetic contexts without Done retain per-call Err polling.
func cachedContextErr(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return ctx.Err()
	}
	select {
	case <-done:
		return ctx.Err()
	default:
		return nil
	}
}

// chargeSkippedRouteWork retains the logical work of a linear scan after its
// irrelevant prefix has been found by binary search. Nonstandard budgets and
// synthetic contexts retain exact step-by-step accounting and cancellation.
func chargeSkippedRouteWork(guard workBudget, amount int) error {
	if guard == nil || amount == 0 {
		return nil
	}
	search, ok := guard.(*routeSearchWorkGuard)
	if !ok || search.done == nil || search.aggregate != nil && !search.deferAggregate {
		for range amount {
			if err := guard.step(); err != nil {
				return err
			}
		}
		return nil
	}
	units := uint64(amount)
	if search.used >= search.limit {
		return search.step()
	}
	if remaining := search.limit - search.used; units > remaining {
		if err := search.add(remaining); err != nil {
			return err
		}
		return search.step()
	}
	return search.add(units)
}
