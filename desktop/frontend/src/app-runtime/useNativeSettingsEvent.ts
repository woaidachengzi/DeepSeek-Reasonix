import { useEffect } from "react";
import { useAppNavigationStore } from "../store/appNavigation";
import { onDesktopOpenSettings } from "../lib/bridge";

export function useNativeSettingsEvent(input: {
  closeTransientOverlays: () => void;
  setSettingsTarget: (target: ReturnType<typeof useAppNavigationStore.getState>["lastSettingsTarget"]) => void;
}) {
  const { closeTransientOverlays, setSettingsTarget } = input;
  useEffect(() => {
    return onDesktopOpenSettings(() => {
      closeTransientOverlays();
      setSettingsTarget(useAppNavigationStore.getState().lastSettingsTarget);
    });
  }, [closeTransientOverlays, setSettingsTarget]);
}
