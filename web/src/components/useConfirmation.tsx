import { AlertDialog, DialogContainer } from "@react-spectrum/s2";
import { useState } from "react";
import { useTranslation } from "react-i18next";

type Confirmation = Readonly<{ title: string; description: string; confirmLabel: string; action: () => void }>;
export function useConfirmation() {
  const { t } = useTranslation();
  const [request, setRequest] = useState<Confirmation | null>(null);
  const dialog = (
    <DialogContainer onDismiss={() => setRequest(null)}>
      {request ? <AlertDialog title={request.title} variant="destructive" cancelLabel={t("drafts.cancel")}
        primaryActionLabel={request.confirmLabel} autoFocusButton="cancel"
        onPrimaryAction={() => { request.action(); setRequest(null); }}>
        {request.description}
      </AlertDialog> : null}
    </DialogContainer>
  );
  return { confirm: setRequest, dialog };
}
