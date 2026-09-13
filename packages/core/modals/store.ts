"use client";

import { create } from "zustand";

type ModalType =
  | "create-issue"
  | "quick-create-issue"
  | "create-project"
  | "create-squad"
  | "feedback"
  | "issue-set-parent"
  | "issue-add-child"
  | "issue-delete-confirm"
  | "issue-run-confirm"
  | null;

export type IssueLimitRecoveryReason = "issue_limit" | "autopilot_quota";

interface ModalStore {
  modal: ModalType;
  data: Record<string, unknown> | null;
  modalInstanceId: number | null;
  issueLimitRecoveryWorkspaceId: string | null;
  issueLimitRecoveryReason: IssueLimitRecoveryReason;
  open: (modal: NonNullable<ModalType>, data?: Record<string, unknown> | null) => void;
  close: (expectedInstanceId?: number) => void;
  showIssueLimitRecovery: (
    workspaceId: string,
    reason?: IssueLimitRecoveryReason,
  ) => void;
  dismissIssueLimitRecovery: () => void;
}

let nextModalInstanceId = 1;

function notifyRunConfirmCancelled(
  modal: ModalType,
  data: Record<string, unknown> | null,
) {
  if (modal !== "issue-run-confirm") return;
  const canCancel = data?.canCancel;
  if (typeof canCancel === "function") {
    try {
      if (!canCancel()) return;
    } catch {
      // A broken lifecycle observer must not emit a false cancellation.
      return;
    }
  }
  const onCancelled = data?.onCancelled;
  if (typeof onCancelled !== "function") return;
  try {
    onCancelled();
  } catch {
    // A modal completion observer must never prevent the store from closing.
  }
}

export const useModalStore = create<ModalStore>((set, get) => ({
  modal: null,
  data: null,
  modalInstanceId: null,
  issueLimitRecoveryWorkspaceId: null,
  issueLimitRecoveryReason: "issue_limit",
  open: (modal, data = null) => {
    const previous = get();
    set({ modal, data, modalInstanceId: nextModalInstanceId++ });
    notifyRunConfirmCancelled(previous.modal, previous.data);
  },
  close: (expectedInstanceId) => {
    const previous = get();
    if (
      expectedInstanceId !== undefined &&
      previous.modalInstanceId !== expectedInstanceId
    ) {
      return;
    }
    set({ modal: null, data: null, modalInstanceId: null });
    notifyRunConfirmCancelled(previous.modal, previous.data);
  },
  showIssueLimitRecovery: (workspaceId, reason = "issue_limit") =>
    set({
      issueLimitRecoveryWorkspaceId: workspaceId,
      issueLimitRecoveryReason: reason,
    }),
  dismissIssueLimitRecovery: () =>
    set({
      issueLimitRecoveryWorkspaceId: null,
      issueLimitRecoveryReason: "issue_limit",
    }),
}));
