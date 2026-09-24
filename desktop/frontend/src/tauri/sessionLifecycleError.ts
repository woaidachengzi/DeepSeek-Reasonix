export type SessionLifecycleFailure = "missing" | "deleting" | "deleted";

/** Only the bridge's allowlisted, status-bound lifecycle markers are trusted. */
export function sessionLifecycleFailure(cause: unknown): SessionLifecycleFailure | undefined {
  const message = cause instanceof Error ? cause.message : typeof cause === "string" ? cause : "";
  const match = /^desktop bridge request failed with status 409 \(session_(missing|deleting|deleted)\)$/.exec(message);
  return match?.[1] as SessionLifecycleFailure | undefined;
}

export function sessionLifecycleNotice(cause: unknown): string | undefined {
  switch (sessionLifecycleFailure(cause)) {
    case "missing":
      return "该会话的文件已不在。不会创建同名空会话；请选择其他对话。";
    case "deleting":
      return "该会话仍在删除中，不能重新打开。请稍后重试删除。";
    case "deleted":
      return "该会话已删除，不能重新打开。";
    default:
      return undefined;
  }
}
