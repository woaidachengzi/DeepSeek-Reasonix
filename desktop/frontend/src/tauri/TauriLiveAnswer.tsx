import { Markdown } from "../components/Markdown";
import type { LiveAnswer } from "./liveProgress";

export function TauriLiveAnswer({ answer, sessionId }: { answer: LiveAnswer; sessionId: string }) {
  return answer.segments.map((text, index) => <div className="tauri-message__live-segment" key={`${sessionId}:${index}`}>
    <Markdown text={text} streaming={answer.streaming && index === answer.segments.length - 1} cacheKey={`${sessionId}:stream:${index}`} />
  </div>);
}
