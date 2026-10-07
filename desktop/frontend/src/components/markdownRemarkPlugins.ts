import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import { remarkMathPolicy } from "./remarkMathPolicy";
import { remarkLocalPathLinks } from "../lib/localPathLinks";
import { remarkSourceReferences } from "../lib/sourceReference";

// One shared parser policy keeps live Markdown and session exports identical.
export const reasonixRemarkPlugins = [remarkGfm, remarkMath, remarkMathPolicy, remarkLocalPathLinks, remarkSourceReferences];
export { reasonixRehypePlugins } from "./rehypeReasonixKatex";
