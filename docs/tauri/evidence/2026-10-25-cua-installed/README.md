# Installed Cua integration — 2026-10-25

The current `/Applications/Reasonix Tauri.app` was upgraded to the Cua edition
0.1.61 with a verified local backup of each previous App. Installation replaced
only the application bundle. Configuration, provider credentials, existing
project folders and conversation storage were not edited by the installer.
The app was opened through its ordinary UI and retained the existing catalog.

## Installed artifact

[installation-0161.json](installation-0161.json) records the installation path,
backup path, version, signature verification and binary hashes. All three
executables in the read-only mounted DMG matched the installed App, and the
runtime's hash matches the previously accepted Cua Driver 0.38.0 binary.
The Cua source remains pinned to
`e754cfec69c41400eb2aabde716c27d15607b2f0`; all third-party notices remain bundled.

The user explicitly approved restoring Reasonix Accessibility and Screen
Recording. Old ad hoc signing records were removed through System Settings,
the current installed App was re-added and restarted, and system authentication
was completed by the user. No TCC database edits or copied permission grants
were used. System Settings was restarted when its file picker retained an old
file identity after the atomic application replacement.

## Issue found and repaired during acceptance

The first installed Cua build successfully filled Chinese text, selected an
option and delivered one trusted click through the actual configured chat model.
A local fixture response-helper naming collision prevented the navigation page
from returning HTTP. The model correctly reported the error. The fixture was
fixed and now verifies a real HTTP 200 response before launching the test page.

Restarting the disposable Chrome instance then exposed a product defect:
Cua's `apps` could list the new PID, but the Go host's `Target` rejected it before
the consent prompt. The host used `NSWorkspace.runningApplications` as an identity
source. Apple's installed SDK header documents that this catalog changes only
when the Cocoa main run loop runs in a common mode; the Go bridge has no Cocoa
main run loop. The 0.1.61 host instead resolves the exact PID with
`NSRunningApplication.runningApplicationWithProcessIdentifier:`. The same bundle
ID and launch-time checks still run before and after consent, and the Rust worker
independently verifies the approved incarnation. The private identity read is not
a model-visible action. The stale-catalog and replaced-process regression tests
passed alongside the Cua/ComputerUse suite across five Go packages;
[go-tests.log](go-tests.log) and [initial-chat-evidence.jsonl](initial-chat-evidence.jsonl).

## Final installed-app acceptance

The final run uses a fresh normal Reasonix chat with the existing configured
`deepseek/deepseek-flash` model and a disposable, account-free Chrome profile
launched **after** Reasonix 0.1.61 was already running. It traverses the ordinary
capability router, tool approval UI, app-access approval, independent browser
profile approval, Go bridge, private Cua worker and typed browser route.

The final run passed. [acceptance.json](acceptance.json) and
[final-chat-evidence.jsonl](final-chat-evidence.jsonl) retain the actual routed
requests and results; [fixture-final-state.json](fixture-final-state.json) is the
independent page-event oracle. The runtime reported `engine=cua`, driver `0.38.0`
and both system permissions granted. All 24 desktop requests were scoped to
the new test PID 49919 when a PID was required.

| Postcondition | Verified result |
| --- | --- |
| Chinese replacement | `Reasonix Cua installed 中文 2026` in DOM and page state |
| Native HTML option selection | `中文选项`, value `zh`, in DOM and page state |
| One trusted click | Exactly one browser-click request and page count `1`; event `isTrusted=true` |
| Same-tab navigation | Destination URL plus heading `本地导航验收完成` read back from DOM |

Merely receiving a navigation request at the fixture is not counted as success;
the destination DOM was verified as well. The model also used its internal goal
completion tool. All desktop actions used `computer_use`; no shell or alternate
UI tool was used. The run handled ordinary setup/stale-ref refusals by inspecting
the returned state, obtaining current refs and verifying postconditions.

[cleanup.json](cleanup.json) confirms all three temporary browser processes
stopped, the final task's Cua worker was reaped, and the installed Reasonix host
and bridge remain running. The final chat is
`tauri-tauri-e6f5b129-557e-4220-81ad-17553c493f05` in the `reasonix-tauri` project.

## Scope

The configured model currently has no image-understanding model. Native window
and tab images can still be returned and shown in PiP; the chat acceptance uses
actual AX/DOM text and does not claim visual interpretation. This does not test
canvas-only workflows or production email accounts. The earlier 18 driver-level
browser checks remain in [the previous evidence](../2026-10-25-cua-browser-actions/README.md).
Windows/Linux and Apple notarization are not part of this local macOS result.
