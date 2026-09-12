# Keep the display output on while locked

This optional workaround leaves Linux's display output on after Omarchy locks.
It is separate from the Monux widget installer and does not change DDC input
switching or the automatic lock timeout.

On a shared Dell P2415Q connected to Linux over DisplayPort and a Mac over
HDMI, waking Linux repeatedly made the Mac's other, directly connected display
briefly go black. Keeping Linux's output on during lock appeared to eliminate
the symptom in a manual test. This identifies output power cycling as a likely
trigger; it does not establish the monitor's internal cause or guarantee a fix
for other setups.

## Apply

Run from the repository root in an unlocked Omarchy session:

```bash
bash plugins/omarchy/keep-display-on.sh enable
```

Requires Python 3 and Omarchy's plugin clone command. If necessary, the script
clones `omarchy.lock` to `~/.config/omarchy/plugins/<username>.lock` and enables
that clone. An existing clone is reused. The only QML change is:

```diff
-    command: ["bash", "-c", "omarchy-brightness-keyboard off; omarchy-brightness-display off"]
+    command: ["bash", "-c", "omarchy-brightness-keyboard off"]
```

The script backs up the QML before changing it and rescans plugins. It refuses
unrecognized command layouts. System-owned Omarchy files remain untouched.

Lock Linux, wait at least ten seconds, then move the mouse or press a key and
observe the Mac's other display. The Linux lock screen should remain lit when
its input is selected. Authentication still applies. This does not prevent
system suspend, manual display power-off, or other programs disabling output.

Keeping the Linux input selected increases backlight hours and power use
compared with blanking the display. When the monitor is showing the Mac input,
Linux simply continues supplying its separate video signal.

## Restore automatic display blanking

```bash
bash plugins/omarchy/keep-display-on.sh disable
```

This restores only the removed display-off command, preserving other edits to
the clone. It keeps the local lock plugin enabled. To return to the packaged
lock plugin instead, run `omarchy plugin enable omarchy.lock`.

A differently named local lock clone can be passed as the second argument.
After Omarchy upgrades, review the clone against the updated built-in plugin;
local clones do not automatically receive upstream lock-screen changes.
