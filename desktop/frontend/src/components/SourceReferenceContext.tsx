import { createContext } from "react";
import type { SourceReference } from "../lib/sourceReference";

// Only a workspace-owning surface can opt into source previews. Wails and
// ordinary web/mail links retain their existing native opener behavior.
export const SourceReferenceContext = createContext<((reference: SourceReference) => void) | null>(null);
