# HTTP history paging

`GET /history` without pagination parameters retains its legacy JSON array.
The built-in Web frontend requests `GET /history?limit=200`, which returns:

```json
{"messages":[],"sessionKey":"...","startIndex":0,"endIndex":0,"totalMessages":0,"hasMoreBefore":false}
```

Use `beforeCursor` from a response as `?limit=200&cursor=...` to load earlier
messages. Cursors bind the source session and boundary entry. Appending new
messages does not move older pages; replacing a session or rewriting the
boundary returns HTTP 409 and requires reloading the newest page. A cursor is
an index/identity check, not a snapshot of every earlier entry. Editing an
earlier message without changing the boundary can appear on the next read.

`limit` is a soft target of 1–500 stored messages. A page expands backward to
keep a contiguous tool-call/result batch together, up to 500 stored messages;
a larger indivisible batch refuses rather than dropping its call. `before=N`
is also accepted as an exclusive stored-message index, but must not split a
tool batch. Manual offsets conservatively refuse tool-result and local/pinned
metadata boundaries; use returned cursors for those generated boundaries.
Indices count the stored transcript, including hidden metadata, not filtered
display rows. Empty pages can therefore still have earlier data.
Page-specific ETags include the bounds and current total as well as content.

An explicit `session` selector on a paged request never falls back to the
foreground. It can select the current session or an externally owned/mirrored
session available through the existing ownership resolver. Unavailable or
out-of-root selectors return 404. Cursor validation does not authorize a file
read or a session switch.

Selection happens before display projection and JSON encoding. Slow transfer
does not hold the foreground binding lock. Controller snapshots still copy the
raw message slice, and mirrored/spectator reads still load the whole transcript
from disk; this change does not claim bounded disk parsing or snapshot copying.
One unusually large stored message may also dominate a page's byte size.

The Web frontend starts with the recent page and offers “Load earlier
messages.” Earlier pages prepend without recreating existing message nodes.
Request identity and selection generations reject stale responses and stale
switch callbacks; a running turn cancels hydration so it cannot overwrite live
output. Interrupted hydration offers an explicit retry after the turn finishes,
and failed session mutations restore that entry point. A failed/stale page
offers retry, with stale cursors restarting from the newest page.
