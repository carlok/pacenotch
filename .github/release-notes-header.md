**Which file?**

| Platform | GUI (tray and window) | Terminal |
|---|---|---|
| macOS | `pacenotch-gui_macos_universal.zip` (the `.app`) | `pacenotch_darwin_arm64`, `pacenotch_darwin_amd64` |
| Windows | `pacenotch-gui_windows_amd64.exe` | `pacenotch_windows_amd64.exe` |
| Linux | `pacenotch-gui_linux_amd64` | `pacenotch_linux_amd64` |

First run: the macOS app is signed ad hoc, not notarized, so after a download run `xattr -d com.apple.quarantine pacenotch.app` once. On Windows, SmartScreen may warn about an unrecognised app: choose *More info*, then *Run anyway*. See the [README](https://github.com/carlok/pacenotch#install) for details.

---
