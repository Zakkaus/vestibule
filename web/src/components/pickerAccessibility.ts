import type { FocusableRefValue } from "@react-types/shared";
import { useEffect, useState } from "react";

type PickerSaveGuardProps = Readonly<{
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  ref: (ref: FocusableRefValue<HTMLButtonElement> | null) => void;
}>;

// S2 1.7.1 overwrites aria-describedby; callers retain external help IDs with SelectContext.
// S2 1.7.1 filters aria-disabled from the trigger; isDisabled would remove save-time focus.
// Must not be combined with caller-controlled isOpen or defaultOpen.
export function usePickerSaveGuard(saving: boolean): PickerSaveGuardProps {
  const [isOpen, setIsOpen] = useState(false);
  useEffect(() => {
    if (saving) setIsOpen(false);
  }, [saving]);
  return {
    isOpen: !saving && isOpen,
    onOpenChange: (open: boolean): void => {
      if (!saving) setIsOpen(open);
    },
    ref: (ref: FocusableRefValue<HTMLButtonElement> | null): void => {
      const button = ref?.UNSAFE_getDOMNode();
      if (saving) button?.setAttribute("aria-disabled", "true");
      else button?.removeAttribute("aria-disabled");
    }
  };
}
