import type { Locale } from "../lib/i18n";

interface FileRevertCopy {
  review: string;
  unavailable: string;
  conflictNotice: string;
  cancel: string;
  confirm: string;
  overwrite: string;
  working: string;
  previewFailed: string;
  failed: string;
  done: string;
  undo: string;
  undoDescription: string;
  undone: string;
  undoFailed: string;
}

interface CheckpointCopy {
  fileRevert: FileRevertCopy;
  tab: string;
  empty: string;
  review: string;
  description: string;
  unavailable: string;
  coverageWarning: string;
  coverageConfirm: string;
  confirm: string;
  done: string;
  failed: string;
}

export const tauriWorkspaceRecoveryCopy: Record<Locale, CheckpointCopy> = {
  en: {
    fileRevert: {
      review: "Review file restore",
      unavailable: "This file cannot be restored. Review the checkpoint or choose another file.",
      conflictNotice: "The file changed after Reasonix wrote it. Restoring will overwrite its current contents. Conflict:",
      cancel: "Cancel",
      confirm: "Restore file",
      overwrite: "Overwrite current file",
      working: "Checking or restoring file…",
      previewFailed: "Could not review the restore. Refresh changes and try again.",
      failed: "Could not restore the file. Refresh changes and review it again.",
      done: "File restored to its state before this session first changed it.",
      undo: "Undo workspace restore",
      undoDescription: "Restores affected files to their contents before the last restore. Undo is refused if a file changed again.",
      undone: "Workspace restore undone.",
      undoFailed: "Could not undo the restore. Check affected files and refresh before retrying.",
    },
    tab: "Checkpoints",
    empty: "This session has no checkpoints yet.",
    review: "Review workspace file rewind",
    description: "Restores files changed in this turn or later to their state before that turn. The conversation stays as it is.",
    unavailable: "These files cannot be restored. Resolve the conflict or choose another checkpoint.",
    coverageWarning: "Some workspace changes may not be covered by these checkpoints:",
    coverageConfirm: "I understand that uncovered changes may remain after the restore.",
    confirm: "Restore listed files",
    done: "Workspace files restored. The conversation is unchanged.",
    failed: "Could not restore the workspace files. Refresh checkpoints and review the plan again.",
  },
  zh: {
    fileRevert: {
      review: "确认恢复文件",
      unavailable: "此文件无法恢复，请检查会话检查点或选择其他文件。",
      conflictNotice: "Reasonix 写入后文件又发生变化。继续恢复会覆盖当前内容。冲突：",
      cancel: "取消",
      confirm: "恢复文件",
      overwrite: "覆盖当前文件",
      working: "正在检查或恢复文件…",
      previewFailed: "无法检查恢复方案，请刷新变更后重试。",
      failed: "无法恢复文件，请刷新变更并重新确认。",
      done: "文件已恢复到本轮会话首次修改前的状态。",
      undo: "撤销工作区恢复",
      undoDescription: "将受影响文件还原到上次恢复前；若文件之后又被修改，撤销会被拒绝。",
      undone: "已撤销工作区恢复。",
      undoFailed: "无法撤销恢复，请检查受影响文件并刷新后重试。",
    },
    tab: "检查点",
    empty: "本轮会话暂无检查点。",
    review: "确认工作区文件回滚",
    description: "将此轮及之后修改的文件恢复到该轮开始前；对话记录保持不变。",
    unavailable: "这些文件无法恢复，请解决冲突或选择其他检查点。",
    coverageWarning: "部分工作区改动可能未被检查点覆盖：",
    coverageConfirm: "我了解恢复后未覆盖的改动可能仍会保留。",
    confirm: "恢复所列文件",
    done: "工作区文件已恢复，对话记录未更改。",
    failed: "无法恢复工作区文件，请刷新检查点并重新确认。",
  },
  "zh-TW": {
    fileRevert: {
      review: "確認還原檔案",
      unavailable: "此檔案無法還原，請檢查會話檢查點或選擇其他檔案。",
      conflictNotice: "Reasonix 寫入後檔案再次變更。繼續還原會覆蓋目前內容。衝突：",
      cancel: "取消",
      confirm: "還原檔案",
      overwrite: "覆蓋目前檔案",
      working: "正在檢查或還原檔案…",
      previewFailed: "無法檢查還原方案，請重新整理變更後再試。",
      failed: "無法還原檔案，請重新整理變更並再次確認。",
      done: "檔案已還原到本輪會話首次修改前的狀態。",
      undo: "撤銷工作區還原",
      undoDescription: "將受影響檔案還原到上次操作前；若檔案之後再次變更，撤銷會被拒絕。",
      undone: "已撤銷工作區還原。",
      undoFailed: "無法撤銷還原，請檢查受影響檔案並重新整理後再試。",
    },
    tab: "檢查點",
    empty: "本輪會話暫無檢查點。",
    review: "確認工作區檔案回復",
    description: "將此輪及之後修改的檔案還原到該輪開始前；對話紀錄保持不變。",
    unavailable: "這些檔案無法還原，請解決衝突或選擇其他檢查點。",
    coverageWarning: "部分工作區變更可能未被檢查點涵蓋：",
    coverageConfirm: "我了解還原後未涵蓋的變更可能仍會保留。",
    confirm: "還原所列檔案",
    done: "工作區檔案已還原，對話紀錄未變更。",
    failed: "無法還原工作區檔案，請重新整理檢查點並再次確認。",
  },
};
