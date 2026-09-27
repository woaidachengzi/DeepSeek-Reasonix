import assert from "node:assert/strict";
import { filterWorkbenchProjectGroups, groupWorkbenchSessions, needsFirstMessageTitle, titleFromFirstUser } from "../tauri/workbenchSessions";

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

const savedFolders = groupWorkbenchSessions(
  [{ sessionId: "active", workspaceRoot: "/work/active/", title: "Recent" }],
  [
    { root: "/work/active", title: "Pinned project title" },
    { root: "/work/empty", title: "Empty project" },
  ],
);
assert.deepEqual(savedFolders.map(group => group.key), ["/work/active", "/work/empty"]);
assert.equal(savedFolders[0].label, "Pinned project title");
assert.deepEqual(savedFolders[0].sessions.map(session => session.sessionId), ["active"]);
assert.equal(savedFolders[1].label, "Empty project");
assert.deepEqual(savedFolders[1].sessions, []);

assert.deepEqual(filterWorkbenchProjectGroups(groups, "LATEST").map(group => [group.key, group.sessions.length]), [["/work/a/project", 1]]);
assert.deepEqual(filterWorkbenchProjectGroups(groups, "project · b").map(group => [group.key, group.sessions.length]), [["/work/b/project", 1]]);
assert.deepEqual(filterWorkbenchProjectGroups(savedFolders, "empty project").map(group => [group.key, group.sessions.length]), [["/work/empty", 0]]);
assert.deepEqual(filterWorkbenchProjectGroups(groups, "not-found"), []);
assert.equal(filterWorkbenchProjectGroups(groups, " ").length, groups.length);

assert.equal(titleFromFirstUser("修复登录失败\n请加回归测试"), "修复登录失败 请加回归测试");
assert.equal(titleFromFirstUser("请处理 😀".repeat(30))?.endsWith("…"), true);
assert.equal(titleFromFirstUser("   "), undefined);
assert.equal(titleFromFirstUser("@.reasonix/attachments/report.pdf"), "文件对话");
assert.equal(titleFromFirstUser("现在使用的这个模型，还需要搭配本地图片的mcp吗，/Users/jerry/vision-bridge"), "现在使用的这个模型，还需要搭配本地图片的mcp吗");
assert.equal(needsFirstMessageTitle("新的会话"), true);
assert.equal(needsFirstMessageTitle("新对话"), true);
assert.equal(needsFirstMessageTitle("手动命名"), false);
console.log("tauri workbench project grouping and first-turn titles: OK");
