import assert from "node:assert/strict";
import { groupWorkbenchSessions, titleFromFirstUser } from "../tauri/workbenchSessions";

const groups = groupWorkbenchSessions([
  { sessionId: "new-a", workspaceRoot: "/work/a/project", title: "Latest" },
  { sessionId: "new-b", workspaceRoot: "/work/b/project/" },
  { sessionId: "old-a", workspaceRoot: "/work/a/project" },
  { sessionId: "global" },
]);
assert.deepEqual(groups.map(group => group.key), ["/work/a/project", "/work/b/project", ""]);
assert.deepEqual(groups[0].sessions.map(session => session.sessionId), ["new-a", "old-a"]);
assert.equal(groups[0].label, "project · a");
assert.equal(groups[1].label, "project · b");
assert.equal(groups[2].label, "");
assert.equal(groups[2].root, undefined);
assert.equal(groupWorkbenchSessions([{ sessionId: "root", workspaceRoot: "/" }])[0].key, "/");
assert.equal(titleFromFirstUser("修复登录失败\n请加回归测试"), "修复登录失败 请加回归测试");
assert.equal(titleFromFirstUser("请处理 😀".repeat(30))?.endsWith("…"), true);
assert.equal(titleFromFirstUser("   "), undefined);
assert.equal(titleFromFirstUser("@.reasonix/attachments/report.pdf"), "文件对话");
console.log("tauri workbench project grouping and first-turn titles: OK");
