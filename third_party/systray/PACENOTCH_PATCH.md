# Local patch to fyne.io/systray

Vendored from `fyne.io/systray v1.12.3-0.20260810170012-af4e8e793ec4`, the version
required by `fyne.io/fyne/v2 v2.8.1`, and wired in with a `replace` in the top-level
`go.mod`. The license is unchanged (Apache 2.0, see `LICENSE`).

## Change

`systray_darwin.m`, in `setIcon` and `setMenuItemIcon`:

```diff
-    [image setSize:NSMakeSize(16, 16)];
+    [image setSize:NSMakeSize(image.size.height > 0 ? 16 * image.size.width / image.size.height : 16, 16)]; // pacenotch: keep the aspect ratio
```

Upstream forces every icon to 16×16 points, which squashes pacenotch's 2:1 menu bar icon
(two horizontal bars with a vertical pace notch). With the patch, icons stay 16 pt tall and
keep their aspect ratio; square icons behave exactly as before.

## Upgrading Fyne

Re-vendor the systray version the new Fyne requires, re-apply the two-line change, and
update the version above. Drop this directory if upstream accepts an equivalent change.
