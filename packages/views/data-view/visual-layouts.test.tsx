import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DataViewCalendar } from "./calendar-view";
import { DataViewGallery } from "./gallery-view";
const row = { id: "1", title: "Sample", date: "2026-09-18" };
const capabilities = {
  layouts: ["calendar", "gallery"],
  editing: "adapter",
  sorting: "none",
  sideEffects: "none",
} as const;
const props = {
  rows: [row],
  rowId: (value: typeof row) => value.id,
  title: (value: typeof row) => value.title,
  date: (value: typeof row) => value.date,
  anchor: "2026-09-18",
  period: "week" as const,
  onAnchorChange: vi.fn(),
  onPeriodChange: vi.fn(),
  capabilities,
  labels: {
    month: "Month",
    week: "Week",
    date: "Date",
    next: "Next",
    previous: "Previous",
    unscheduled: "Unscheduled",
  },
};
afterEach(cleanup);
it("writes calendar drops through the adapter and hides editing when denied", () => {
  const move = vi.fn();
  const view = render(<DataViewCalendar {...props} onMoveDate={move} />);
  fireEvent.drop(screen.getByRole("region", { name: "2026-09-19" }), {
    dataTransfer: { getData: () => row.id },
  });
  expect(move).toHaveBeenCalledWith(row, "2026-09-19");
  view.rerender(
    <DataViewCalendar
      {...props}
      capabilities={{ ...capabilities, editing: "none" }}
      onMoveDate={move}
    />,
  );
  expect(screen.queryByLabelText("Date: Sample")).toBeNull();
  fireEvent.drop(screen.getByRole("region", { name: "2026-09-19" }), {
    dataTransfer: { getData: () => row.id },
  });
  expect(move).toHaveBeenCalledTimes(1);
});
it("renders configured gallery fields and rejects non-http covers", () => {
  render(
    <DataViewGallery
      rows={[row]}
      rowId={props.rowId}
      title={props.title}
      capabilities={capabilities}
      cover={() => "javascript:alert(1)"}
      fields={[
        { id: "date", label: "Schedule", kind: "date", value: props.date },
      ]}
    />,
  );
  expect(screen.getByRole("article").textContent).toContain("2026-09-18");
  expect(screen.queryByRole("img")).toBeNull();
});

it("retains a failed cell draft through a server refresh until an explicit merge retry", async () => {
  const save = vi
    .fn()
    .mockRejectedValueOnce(new Error("conflict"))
    .mockResolvedValueOnce({});
  const { DataFieldEditor } = await import("./field-editor");
  const props = {
    label: "Title",
    kind: "text" as const,
    save,
    labels: {
      current: "Current",
      retry: "Overwrite current",
      discard: "Discard",
    },
  };
  const view = render(<DataFieldEditor {...props} value="Original" />);
  fireEvent.focus(screen.getByRole("textbox"));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Draft" } });
  fireEvent.blur(screen.getByRole("textbox"));
  await screen.findByRole("alert");
  view.rerender(<DataFieldEditor {...props} value="Remote" />);
  expect(screen.getByRole("textbox")).toHaveValue("Draft");
  expect(screen.getByText("Current: Remote")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Overwrite current" }));
  expect(save).toHaveBeenLastCalledWith("Draft", "Remote");
});
