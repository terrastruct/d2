# Dev notes

ELKJS bundle is sourced from https://cdn.jsdelivr.net/npm/elkjs@0.3.0/lib/ .

By itself, it does not work, due to incompatability with the Javascript runner.

It has some code where it tries to set `$wnd` to whatever global is (e.g. self, window,
global), and tries to do global calls like `$wnd.Math()`.

This is not found when run in goja. But `Math()` is indeed set on global.

So I just find and replace all instances to not use a `$wnd` object.
