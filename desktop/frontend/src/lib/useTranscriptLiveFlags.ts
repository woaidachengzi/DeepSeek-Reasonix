import { useMemo } from "react";
import type { LiveStream } from "./useController";
import { NO_LIVE, type TranscriptLiveFlags } from "./transcriptRows";

// Row models depend on stream presence/completion, while descendants consume
// the current text through LiveStreamContext. Text-only growth keeps identity.
export function useTranscriptLiveFlags(live: LiveStream | undefined): TranscriptLiveFlags {
  const id = live?.id;
  const hasAnswerText = Boolean(live?.text.trim());
  const hasReasoning = Boolean(live?.reasoning);
  const reasoningComplete = live?.reasoningComplete;
  return useMemo(() => id ? {
    id, hasAnswerText, hasReasoning, reasoningComplete,
  } : NO_LIVE, [id, hasAnswerText, hasReasoning, reasoningComplete]);
}
