// @vitest-environment node

import { beforeEach, describe, expect, it, vi } from "vitest";
import { useModalStore } from "./store";

beforeEach(() => {
  useModalStore.setState({ modal: null, data: null, modalInstanceId: null });
});

describe("modal store run-confirm completion", () => {
  it("reports cancellation when a run-confirm modal is closed programmatically", () => {
    const onCancelled = vi.fn();
    useModalStore.getState().open("issue-run-confirm", { onCancelled });

    useModalStore.getState().close();

    expect(onCancelled).toHaveBeenCalledTimes(1);
  });

  it("reports cancellation when another modal replaces run-confirm", () => {
    const onCancelled = vi.fn();
    useModalStore.getState().open("issue-run-confirm", { onCancelled });

    useModalStore.getState().open("feedback");

    expect(onCancelled).toHaveBeenCalledTimes(1);
  });

  it("does not report cancellation after confirmation starts submitting", () => {
    const onCancelled = vi.fn();
    useModalStore.getState().open("issue-run-confirm", {
      canCancel: () => false,
      onCancelled,
    });

    useModalStore.getState().open("feedback");

    expect(onCancelled).not.toHaveBeenCalled();
  });

  it("does not treat closing another modal as a run-confirm cancellation", () => {
    const onCancelled = vi.fn();
    useModalStore.getState().open("feedback", { onCancelled });

    useModalStore.getState().close();

    expect(onCancelled).not.toHaveBeenCalled();
  });

  it("does not let a stale modal instance close its replacement", () => {
    useModalStore.getState().open("issue-run-confirm");
    const staleInstanceId = useModalStore.getState().modalInstanceId!;
    useModalStore.getState().open("feedback");
    const currentInstanceId = useModalStore.getState().modalInstanceId;

    useModalStore.getState().close(staleInstanceId);

    expect(useModalStore.getState()).toMatchObject({
      modal: "feedback",
      modalInstanceId: currentInstanceId,
    });
  });
});
