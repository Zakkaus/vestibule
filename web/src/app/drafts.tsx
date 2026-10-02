import { AlertDialog, DialogContainer } from "@react-spectrum/s2";
import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useBlocker } from "react-router-dom";

/* Draft contract: each independently editable draft registers dirty/pending state and a
 * discard callback. The shell blocks routes (including group changes and Back/Forward)
 * and explicit scope reloads while any owner is dirty or writing; cancel keeps the URL.
 * Pending writes cannot be discarded. Once settled, confirm calls every dirty owner's
 * discard before proceeding. Browser unload alone uses the native warning.
 * Conflict owners retain the baseline revision and field delta until explicit discard
 * or reapply onto a fetched revision; reapply never writes without another Save.
 */
type Owner = Readonly<{ dirty: boolean; pending: boolean; discard: () => void }>;

function createRegistry() {
  const owners = new Map<symbol, Owner>();
  const listeners = new Set<() => void>();
  let version = 0;
  const notify = () => { version += 1; listeners.forEach((listener) => listener()); };
  return {
    owners,
    subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
    snapshot: () => version,
    set: (id: symbol, owner: Owner) => { owners.set(id, owner); notify(); },
    remove: (id: symbol) => { owners.delete(id); notify(); }
  };
}

type Registry = {
  owners: Map<symbol, Owner>;
  subscribe: (listener: () => void) => () => void;
  snapshot: () => number;
  set: (id: symbol, owner: Owner) => void;
  remove: (id: symbol) => void;
};
const DraftContext = createContext<{ registry: Registry; requestScopeChange: (action: () => void) => void } | null>(null);

function useDraftContext() {
  const context = useContext(DraftContext);
  if (!context) throw new Error("DraftProvider is required");
  return context;
}

export function useDraftOwner(dirty: boolean, pending: boolean, discard: () => void): void {
  const { registry } = useDraftContext();
  const [id] = useState(() => Symbol());
  const discardRef = useRef(discard);
  useLayoutEffect(() => { discardRef.current = discard; });
  useLayoutEffect(() => {
    registry.set(id, { dirty, pending, discard: () => discardRef.current() });
    return () => registry.remove(id);
  }, [registry, id, dirty, pending]);
}

export function useScopeChange() {
  return useDraftContext().requestScopeChange;
}

function states(registry: Registry) {
  const owners = [...registry.owners.values()];
  return { dirty: owners.some((owner) => owner.dirty), pending: owners.some((owner) => owner.pending) };
}

function DraftGuard({ registry, action, clear }: Readonly<{ registry: Registry; action: (() => void) | null; clear: () => void }>) {
  const { t } = useTranslation();
  useSyncExternalStore(registry.subscribe, registry.snapshot);
  const { dirty, pending } = states(registry);
  const blocker = useBlocker(({ currentLocation, nextLocation }) => (dirty || pending) &&
    (currentLocation.pathname !== nextLocation.pathname ||
      new URLSearchParams(currentLocation.search).get("group") !== new URLSearchParams(nextLocation.search).get("group")));
  const blocked = blocker.state === "blocked";
  const actionHandled = useRef(false);
  useEffect(() => {
    if (!dirty && !pending) return;
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ""; };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty, pending]);
  const cancel = () => { actionHandled.current = true; if (blocked) blocker.reset(); clear(); };
  const discard = () => {
    actionHandled.current = true;
    if (states(registry).pending) return;
    registry.owners.forEach((owner) => { if (owner.dirty) owner.discard(); });
    clear();
    if (blocked) blocker.proceed();
    else action?.();
  };
  return (
    <DialogContainer onDismiss={() => {
      // S2 closes before invoking the action. Defer dismissal so only Escape cancels.
      actionHandled.current = false;
      queueMicrotask(() => { if (!actionHandled.current) cancel(); });
    }}>
      {blocked || action ? (
        <AlertDialog title={t("drafts.leaveTitle")} variant="warning" cancelLabel={t("drafts.cancel")}
          primaryActionLabel={t("drafts.discardContinue")} isPrimaryActionDisabled={pending}
          onCancel={cancel} onPrimaryAction={discard} autoFocusButton="cancel">
          {t(pending ? "drafts.writePending" : "drafts.leaveDescription")}
        </AlertDialog>
      ) : null}
    </DialogContainer>
  );
}

export function DraftProvider({ children }: Readonly<{ children: ReactNode }>) {
  const [registry] = useState(createRegistry);
  const [action, setAction] = useState<(() => void) | null>(null);
  const requestScopeChange = useCallback((next: () => void) => {
    const { dirty, pending } = states(registry);
    if (dirty || pending) setAction(() => next);
    else next();
  }, [registry]);
  const context = useMemo(() => ({ registry, requestScopeChange }), [registry, requestScopeChange]);
  return (
    <DraftContext.Provider value={context}>
      {children}
      <DraftGuard registry={registry} action={action} clear={() => setAction(null)} />
    </DraftContext.Provider>
  );
}
