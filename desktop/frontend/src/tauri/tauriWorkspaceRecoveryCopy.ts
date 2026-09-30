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
  working: string;
  heads: { title: string; current: string; switch: string; switched: string; failed: string; main: string; rewind: string; fork: string; concurrent: string };
  combined: { action: string; review: string; description: string; unavailable: string; confirm: string; done: string; partial: string; failed: string; undo: string; undoDescription: string; undone: string; undoFilesOnly: string; undoSelectionUnknown: string; undoFailed: string };
  legacyFork: { action: string; review: string; description: string; unavailable: string; confirm: string; failed: string };
  conversation: {
    action: string;
    review: string;
    description: string;
    unavailable: string;
    confirm: string;
    done: string;
    failed: string;
    undo: string;
    undoDescription: string;
    undone: string;
  };
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
    working: "Checking or restoring checkpoint…",
    heads: { title: "Conversation versions", current: "Current", switch: "Switch", switched: "Conversation version switched. Workspace files are unchanged.", failed: "Could not switch conversation version. Refresh versions and try again.", main: "Original", rewind: "Rewound", fork: "Branch", concurrent: "Parallel" },
    combined: { action: "Rewind both", review: "Review files and conversation", description: "Creates a new conversation version and restores listed files. If files change before commit, only the conversation may be rewound.", unavailable: "This checkpoint cannot rewind both files and conversation. Resolve conflicts or choose another checkpoint.", confirm: "Rewind files and conversation", done: "Conversation version created and workspace files restored.", partial: "Conversation version created, but file restore did not complete. Check workspace files and refresh checkpoints before retrying.", failed: "Could not rewind files and conversation. Refresh checkpoints and review the plan again.", undo: "Undo combined rewind", undoDescription: "Restores the files. Also returns to the previous conversation if no message was added to the new version.", undone: "Files restored and previous conversation selected.", undoFilesOnly: "Files restored. The continued conversation version remains selected; use conversation versions to switch.", undoSelectionUnknown: "Files restored. Refresh conversation versions to confirm the selected version.", undoFailed: "Could not undo the combined rewind. Refresh checkpoints and review the files before retrying." },
    legacyFork: { action: "Fork older conversation", review: "Review older conversation fork", description: "Creates and opens a separate conversation before this turn. The original conversation and workspace files stay unchanged.", unavailable: "This checkpoint cannot create a separate conversation fork.", confirm: "Create and open fork", failed: "Could not create the conversation fork. Refresh checkpoints and try again." },
    conversation: {
      action: "Rewind conversation",
      review: "Review conversation rewind",
      description: "Starts a new conversation version before this turn. Workspace files stay as they are.",
      unavailable: "This conversation cannot be rewound here. Choose a checkpoint with a conversation boundary; the stable app can handle older session formats.",
      confirm: "Create conversation version",
      done: "Conversation rewound to a new version. Workspace files are unchanged.",
      failed: "Could not rewind the conversation. Refresh checkpoints and review it again.",
      undo: "Return to previous conversation",
      undoDescription: "Available until a message is added to the new version.",
      undone: "Returned to the previous conversation.",
    },
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
    working: "正在检查或恢复检查点…",
    heads: { title: "对话版本", current: "当前", switch: "切换", switched: "已切换对话版本，工作区文件未更改。", failed: "无法切换对话版本，请刷新版本后重试。", main: "原始", rewind: "回滚", fork: "分支", concurrent: "并行" },
    combined: { action: "同时回滚", review: "确认文件与对话回滚", description: "创建新的对话版本并恢复所列文件。若提交前文件又发生变化，可能只有对话完成回滚。", unavailable: "此检查点无法同时回滚文件和对话，请解决冲突或选择其他检查点。", confirm: "回滚文件与对话", done: "已创建对话版本并恢复工作区文件。", partial: "已创建对话版本，但文件恢复未完成。请检查工作区文件并刷新检查点后重试。", failed: "无法同时回滚文件和对话，请刷新检查点并重新确认。", undo: "撤销组合回滚", undoDescription: "恢复文件；若新对话版本尚未添加消息，也会返回原对话。", undone: "文件已恢复，并返回原对话。", undoFilesOnly: "文件已恢复；继续过的对话版本仍为当前版本，可从对话版本列表切换。", undoSelectionUnknown: "文件已恢复，请刷新对话版本以确认当前版本。", undoFailed: "无法撤销组合回滚，请刷新检查点并检查文件后重试。" },
    legacyFork: { action: "旧格式分叉", review: "确认旧格式对话分叉", description: "在此轮开始前创建并打开独立会话；原会话和工作区文件保持不变。", unavailable: "此检查点无法创建独立对话分叉。", confirm: "创建并打开分叉", failed: "无法创建对话分叉，请刷新检查点后重试。" },
    conversation: {
      action: "回滚对话",
      review: "确认对话回滚",
      description: "在此轮开始前创建新的对话版本；工作区文件保持不变。",
      unavailable: "当前无法回滚此对话，请选择有对话边界的检查点；旧格式会话可使用稳定版处理。",
      confirm: "创建对话版本",
      done: "已创建回滚后的对话版本，工作区文件未更改。",
      failed: "无法回滚对话，请刷新检查点并重新确认。",
      undo: "返回原对话版本",
      undoDescription: "在新版本继续发送消息前可返回原对话。",
      undone: "已返回原对话版本。",
    },
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
    working: "正在檢查或還原檢查點…",
    heads: { title: "對話版本", current: "目前", switch: "切換", switched: "已切換對話版本，工作區檔案未變更。", failed: "無法切換對話版本，請重新整理版本後再試。", main: "原始", rewind: "回溯", fork: "分支", concurrent: "並行" },
    combined: { action: "同時回溯", review: "確認檔案與對話回溯", description: "建立新的對話版本並還原所列檔案。若提交前檔案再次變更，可能只有對話完成回溯。", unavailable: "此檢查點無法同時回溯檔案和對話，請解決衝突或選擇其他檢查點。", confirm: "回溯檔案與對話", done: "已建立對話版本並還原工作區檔案。", partial: "已建立對話版本，但檔案還原未完成。請檢查工作區檔案並重新整理檢查點後再試。", failed: "無法同時回溯檔案和對話，請重新整理檢查點並再次確認。", undo: "復原組合回溯", undoDescription: "還原檔案；若新對話版本尚未新增訊息，也會返回原對話。", undone: "檔案已還原，並返回原對話。", undoFilesOnly: "檔案已還原；繼續過的對話版本仍為目前版本，可從對話版本清單切換。", undoSelectionUnknown: "檔案已還原，請重新整理對話版本以確認目前版本。", undoFailed: "無法復原組合回溯，請重新整理檢查點並檢查檔案後重試。" },
    legacyFork: { action: "舊格式分支", review: "確認舊格式對話分支", description: "在此輪開始前建立並開啟獨立會話；原會話和工作區檔案保持不變。", unavailable: "此檢查點無法建立獨立對話分支。", confirm: "建立並開啟分支", failed: "無法建立對話分支，請重新整理檢查點後再試。" },
    conversation: {
      action: "回復對話",
      review: "確認對話回復",
      description: "在此輪開始前建立新的對話版本；工作區檔案保持不變。",
      unavailable: "目前無法回復此對話，請選擇有對話邊界的檢查點；舊格式會話可使用穩定版處理。",
      confirm: "建立對話版本",
      done: "已建立回復後的對話版本，工作區檔案未變更。",
      failed: "無法回復對話，請重新整理檢查點並再次確認。",
      undo: "返回原對話版本",
      undoDescription: "在新版本繼續傳送訊息前可返回原對話。",
      undone: "已返回原對話版本。",
    },
  },
};
