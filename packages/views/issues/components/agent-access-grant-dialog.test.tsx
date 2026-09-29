import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { renderWithI18n } from "../../test/i18n";
import type { CommentTriggerPreviewAgent } from "@multica/core/types";
import { AgentAccessGrantDialog, agentsNeedingGrant, useAgentAccessGrantPrompt } from "./agent-access-grant-dialog";

afterEach(cleanup);

const blocked: CommentTriggerPreviewAgent = {
  id: "a1", name: "Planner", source: "mention_agent", reason: "",
  document_access: { runtime_owner_permission: "", max_grant: "edit" },
};
const readsAlready: CommentTriggerPreviewAgent = {
  id: "a2", name: "Reader", source: "mention_agent", reason: "",
  document_access: { runtime_owner_permission: "view", max_grant: "edit" },
};
const fine: CommentTriggerPreviewAgent = {
  id: "a3", name: "Editor", source: "mention_agent", reason: "",
  document_access: { runtime_owner_permission: "edit", max_grant: "" },
};
const onTask: CommentTriggerPreviewAgent = { id: "a4", name: "Task agent", source: "mention_agent", reason: "" };

it("asks only about agents whose run cannot do all the poster could grant", () => {
  expect(agentsNeedingGrant([blocked, readsAlready, fine, onTask]).map((a) => a.id)).toEqual(["a1", "a2"]);
});

it("offers the most the poster may grant, and read-only unless the run reads already", () => {
  const settled: unknown[] = [];
  renderWithI18n(<AgentAccessGrantDialog agents={[blocked, readsAlready]} onSettle={(g) => settled.push(g)} />);
  const planner = screen.getByRole("group", { name: "Planner" });
  expect(planner.querySelectorAll("input[type=radio]")).toHaveLength(3);
  const reader = screen.getByRole("group", { name: "Reader" });
  // Reader's run can already read, so "reading only" is not offered.
  expect(reader.querySelectorAll("input[type=radio]")).toHaveLength(2);
  // Defaults to the most that can be granted; Reader gets nothing extra.
  fireEvent.click(reader.querySelector("input[value=none]")!);
  fireEvent.click(screen.getByRole("button", { name: "Allow and send" }));
  expect(settled).toEqual([[{ agent_id: "a1", permission: "edit" }]]);
});

it("resolves with null when cancelled so the draft is kept", () => {
  const settled: unknown[] = [];
  renderWithI18n(<AgentAccessGrantDialog agents={[blocked]} onSettle={(g) => settled.push(g)} />);
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(settled).toEqual([null]);
});

function Harness({ onAnswer }: { onAnswer: (grants: unknown) => void }) {
  const prompt = useAgentAccessGrantPrompt();
  return (
    <div>
      <button onClick={() => prompt.ask([blocked]).then(onAnswer)}>ask</button>
      <button onClick={() => prompt.ask([fine]).then(onAnswer)}>ask-fine</button>
      {prompt.dialog}
    </div>
  );
}

it("skips the dialog when nothing needs granting and otherwise waits for the answer", async () => {
  const answers: unknown[] = [];
  renderWithI18n(<Harness onAnswer={(g) => answers.push(g)} />);
  await act(async () => { fireEvent.click(screen.getByText("ask-fine")); });
  expect(answers).toEqual([[]]);
  expect(screen.queryByTestId("agent-access-grant-dialog")).toBeNull();
  await act(async () => { fireEvent.click(screen.getByText("ask")); });
  expect(await screen.findByTestId("agent-access-grant-dialog")).toBeTruthy();
  fireEvent.click(screen.getByLabelText("Allow reading only for this run"));
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Allow and send" })); });
  expect(answers).toEqual([[], [{ agent_id: "a1", permission: "view" }]]);
  expect(screen.queryByTestId("agent-access-grant-dialog")).toBeNull();
});
