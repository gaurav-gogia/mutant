# The three DFRWS APAC 2026 topics

These four programs are the hands-on core of the workshop. Each one answers one
segment of the outline. Each stands alone: it imports nothing, and the evidence
it reads is named on one line near its top.

| Outline segment | Program | The question it answers |
|---|---|---|
| Detecting timestomping | [1_timestomp.mut](1_timestomp.mut) | Which file says it was created before it was given its name? |
| Malware hidden in ads | [2a_ad_redirects.mut](2a_ad_redirects.mut) | Which pages did the browser reach through a redirect, and where did each chain start? |
| Malware hidden in ads | [2b_ad_cookies.mut](2b_ad_cookies.mut) | Which sites left a cookie, and which of them did the user never visit? |
| Malicious registry entries | [3_run_keys.mut](3_run_keys.mut) | What starts at logon, since when, and from where? |

## Run one

From the repository root:

```
mutant gen --src examples/workshop/dfrws/1_timestomp.mut --dev
mutant examples/workshop/dfrws/1_timestomp.mu --dev
```

or `.\examples\workshop\run.ps1 dfrws/1_timestomp` (`run.sh` on Linux and
macOS).

## Point it at other evidence

Change the path on the line that names the input. Nothing else needs to change.

| Program | Line to change | What it accepts |
|---|---|---|
| `1_timestomp.mut` | `let disk = ...` | A raw disk image (MBR or GPT), or a raw NTFS volume. Every NTFS partition on the disk is checked. An E01 is refused: convert it to raw first. `let mft` names where the volume's `$MFT` is copied to |
| `2a_ad_redirects.mut` | `let history = ...` | Chrome's `History` database, normally `AppData/Local/Google/Chrome/User Data/Default/History` |
| `2b_ad_cookies.mut` | `let profile = ...` | A Chrome profile directory holding `History` and `Network/Cookies` |
| `3_run_keys.mut` | `let hive = ...` | `NTUSER.DAT` or the `SOFTWARE` hive. A hive with pending transaction logs (`.LOG1`/`.LOG2`) is read as it stands: replay the logs first |

Without its input, each program prints one `missing` line and exits cleanly.
The `.golden` beside it records exactly that line. To make the workshop's own
evidence for these four, and nothing else, run from the repository root:

```
.\examples\workshop\evidence\build_evidence.ps1 -Topic timestomp,ads,registry
./examples/workshop/evidence/build_evidence.sh --topic timestomp,ads,registry
```

Each topic builds alone too: `timestomp` (the disk image), `ads` (History and
Cookies) and `registry` (NTUSER.DAT). None of the three writes an executable
where Defender scans it. [../evidence/README.md](../evidence/README.md) says
what each one holds.

## What each one looks at

- **Timestomping.** NTFS keeps two sets of times for each file.
  `$STANDARD_INFORMATION` is what `SetFileTime` writes, so a timestomping tool
  changes it. `$FILE_NAME` is written when the name is made, and the usual tools
  leave it alone. A creation time in the first that is earlier than the one in
  the second is the finding. A tool that writes both clocks leaves nothing here
  to find.
- **Redirects.** Chrome stores how each visit happened in `transition`. The
  high bits say a visit was a server redirect (`2147483648`, `0x80000000`) or a
  client redirect (`1073741824`, `0x40000000`), and which visit ended a chain
  (`536870912`, `0x20000000`). The program walks each chain back through
  `from_visit` to the visit that started it. Mutant has no hex literals, so the
  bits are written in decimal.
- **Cookies.** A cookie from a site the history never visited was set by
  something embedded in a page the user did visit, such as an ad or a tracker.
  Chrome encrypts cookie values. The host and name are not encrypted, and they
  are all this question needs.
- **Run keys.** A key's last-write time is when it last changed, so it dates the
  persistence. The `odd` rule is one regular expression: ours, not a vendor's.
  Change it and run the program again.
