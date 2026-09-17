/** @vitest-environment jsdom */

import { useState } from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type {
  DataSource,
  DataSourceActionResult,
  DataSourceCellCommand,
  DataSourceField,
} from "@multica/core/data-source";
import {
  DataViewCellEditor,
  type DataViewCellEditorState,
} from "./cell-editor";

type Row = {
  id: string;
  caption: string;
  amount: number;
  bucket: number | undefined;
  flag: boolean;
};

function source(
  execute: (
    command: DataSourceCellCommand<Row>,
  ) => Promise<DataSourceActionResult<Row>>,
): Pick<
  DataSource<
    Row,
    unknown,
    DataSourceCellCommand<Row>,
    DataSourceField<Row>
  >,
  "capabilities" | "execute" | "identity" | "rowId"
> {
  return {
    identity: { workspaceId: "ws:a", namespace: "sample", sourceId: "s::1" },
    capabilities: {
      layouts: ["table"],
      grouping: false,
      hierarchy: false,
      writable: true,
      maxPageSize: 10,
    },
    rowId: (row) => row.id,
    execute,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

const row: Row = {
  id: "root",
  caption: "Original",
  amount: 1,
  bucket: 7,
  flag: true,
};

function textField(): DataSourceField<Row> {
  return {
    id: "caption",
    label: "Caption",
    kind: "text",
    value: (candidate) => candidate.caption,
    sortable: true,
    filterable: false,
    groupable: false,
    canSet: () => true,
    canClear: () => true,
  };
}

describe("DataViewCellEditor", () => {
  it("keeps the edited row as the first CAS baseline and rebases only after failure", async () => {
    const execute = vi
      .fn<(command: DataSourceCellCommand<Row>) => Promise<DataSourceActionResult<Row>>>()
      .mockResolvedValueOnce({
        status: "failed",
        error: new Error("revision conflict"),
      })
      .mockImplementationOnce(async (command) => ({
        status: "accepted",
        row: {
          ...command.row,
          caption:
            command.change.op === "set"
              ? String(command.change.value)
              : "",
        },
      }));
    const view = render(
      <DataViewCellEditor
        source={source(execute)}
        field={textField()}
        row={row}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );
    fireEvent.change(screen.getByLabelText("Caption"), {
      target: { value: "My draft" },
    });
    view.rerender(
      <DataViewCellEditor
        source={source(execute)}
        field={textField()}
        row={{ ...row, caption: "Remote edit", amount: 2 }}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]?.[0].row).toEqual(row);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "revision conflict",
    );
    expect(screen.getByLabelText("Caption")).toHaveValue("My draft");

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(2));
    expect(execute.mock.calls[1]?.[0].row).toEqual({
      ...row,
      caption: "Remote edit",
      amount: 2,
    });
  });

  it("keeps a dirty pending/failed draft across equivalent DTO refreshes", async () => {
    const write = deferred<DataSourceActionResult<Row>>();
    const execute = vi.fn(() => write.promise);
    const firstSource = source(execute);
    const view = render(
      <DataViewCellEditor
        source={firstSource}
        field={textField()}
        row={row}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );
    const input = screen.getByLabelText("Caption");
    fireEvent.change(input, { target: { value: "Draft survives" } });
    fireEvent.submit(input.closest("form")!);
    expect(input).toBeDisabled();

    view.rerender(
      <DataViewCellEditor
        source={source(execute)}
        field={textField()}
        row={{ ...row, caption: "Optimistic", amount: 2 }}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );
    expect(screen.getByLabelText("Caption")).toHaveValue("Draft survives");
    expect(screen.getByLabelText("Caption")).toBeDisabled();

    await act(async () => {
      write.resolve({ status: "failed", error: new Error("write failed") });
      await write.promise;
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("write failed");
    expect(screen.getByLabelText("Caption")).toHaveValue("Draft survives");

    view.rerender(
      <DataViewCellEditor
        source={source(execute)}
        field={textField()}
        row={{ ...row, caption: "Original", amount: 3 }}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );
    expect(screen.getByLabelText("Caption")).toHaveValue("Draft survives");
  });

  it("restores a dirty and pending editor after virtualization-style unmounts", async () => {
    const write = deferred<DataSourceActionResult<Row>>();
    const execute = vi.fn(() => write.promise);
    function Harness() {
      const [visible, setVisible] = useState(true);
      const [persisted, setPersisted] = useState<
        DataViewCellEditorState<Row> | undefined
      >();
      return (
        <>
          <button type="button" onClick={() => setVisible((current) => !current)}>
            {visible ? "Hide" : "Show"}
          </button>
          {visible ? (
            <DataViewCellEditor
              source={source(execute)}
              field={textField()}
              row={row}
              persistedState={persisted}
              onStateChange={(state) => setPersisted(state ?? undefined)}
              saveLabel="Save"
              clearLabel="Clear"
            />
          ) : null}
        </>
      );
    }
    render(<Harness />);

    fireEvent.change(screen.getByLabelText("Caption"), {
      target: { value: "Virtual draft" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Hide" }));
    fireEvent.click(screen.getByRole("button", { name: "Show" }));
    expect(screen.getByLabelText("Caption")).toHaveValue("Virtual draft");

    fireEvent.submit(screen.getByLabelText("Caption").closest("form")!);
    expect(screen.getByLabelText("Caption")).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Hide" }));
    fireEvent.click(screen.getByRole("button", { name: "Show" }));
    expect(screen.getByLabelText("Caption")).toHaveValue("Virtual draft");
    expect(screen.getByLabelText("Caption")).toBeDisabled();

    await act(async () => {
      write.resolve({ status: "failed", error: new Error("write failed") });
      await write.promise;
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("write failed");
    expect(screen.getByLabelText("Caption")).toHaveValue("Virtual draft");
    expect(screen.getByLabelText("Caption")).toBeEnabled();
  });

  it("keeps set-empty distinct from the explicit clear command", async () => {
    const execute = vi.fn(async (command: DataSourceCellCommand<Row>) => ({
      status: "accepted" as const,
      row: {
        ...command.row,
        caption:
          command.change.op === "set" ? String(command.change.value) : "",
      },
    }));
    render(
      <DataViewCellEditor
        source={source(execute)}
        field={textField()}
        row={row}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );
    const input = screen.getByLabelText("Caption");
    fireEvent.change(input, { target: { value: "" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]?.[0].change).toEqual({ op: "set", value: "" });

    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(2));
    expect(execute.mock.calls[1]?.[0].change).toEqual({ op: "clear" });
  });

  it("maps select option ids to domain values and preserves zero/false writes", async () => {
    const execute = vi.fn(async (_command: DataSourceCellCommand<Row>) => ({
      status: "accepted" as const,
    }));
    const selectField: DataSourceField<Row> = {
      id: "bucket",
      label: "Bucket",
      kind: "select",
      value: (candidate) => candidate.bucket,
      sortable: true,
      filterable: false,
      groupable: true,
      canSet: () => true,
      canClear: () => true,
      options: [
        { id: "ui-seven", label: "Seven", value: 7 },
        { id: "ui-zero", label: "Zero", value: 0 },
      ],
    };
    const view = render(
      <DataViewCellEditor
        source={source(execute)}
        field={selectField}
        row={row}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );
    expect(screen.getByLabelText("Bucket")).toHaveValue("ui-seven");
    fireEvent.change(screen.getByLabelText("Bucket"), {
      target: { value: "ui-zero" },
    });
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]?.[0].change).toEqual({ op: "set", value: 0 });

    const checkboxField: DataSourceField<Row> = {
      id: "flag",
      label: "Flag",
      kind: "checkbox",
      value: (candidate) => candidate.flag,
      sortable: false,
      filterable: false,
      groupable: false,
      canSet: () => true,
      canClear: () => true,
    };
    view.rerender(
      <DataViewCellEditor
        source={source(execute)}
        field={checkboxField}
        row={row}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );
    fireEvent.click(screen.getByLabelText("Flag"));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(2));
    expect(execute.mock.calls[1]?.[0].change).toEqual({
      op: "set",
      value: false,
    });
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(3));
    expect(execute.mock.calls[2]?.[0].change).toEqual({ op: "clear" });
  });

  it.each([
    { name: "set true", initial: false, action: "toggle", change: { op: "set", value: true } },
    { name: "set false", initial: true, action: "toggle", change: { op: "set", value: false } },
    { name: "clear", initial: true, action: "clear", change: { op: "clear" } },
  ] as const)("retries the original checkbox $name after a final failure", async ({
    initial,
    action,
    change,
  }) => {
    const execute = vi
      .fn<(command: DataSourceCellCommand<Row>) => Promise<DataSourceActionResult<Row>>>()
      .mockResolvedValueOnce({
        status: "failed",
        error: new Error("revision conflict"),
      })
      .mockResolvedValueOnce({ status: "accepted" });
    const checkboxField: DataSourceField<Row> = {
      id: "flag",
      label: "Flag",
      kind: "checkbox",
      value: (candidate) => candidate.flag,
      sortable: false,
      filterable: false,
      groupable: false,
      canSet: () => true,
      canClear: () => true,
    };
    render(
      <DataViewCellEditor
        source={source(execute)}
        field={checkboxField}
        row={{ ...row, flag: initial }}
        saveLabel="Save"
        clearLabel="Clear"
        retryLabel="Retry"
      />,
    );

    fireEvent.click(
      action === "toggle"
        ? screen.getByLabelText("Flag")
        : screen.getByRole("button", { name: "Clear" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "revision conflict",
    );
    expect(execute).toHaveBeenCalledOnce();
    expect(execute.mock.calls[0]?.[0].change).toEqual(change);

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(2));
    expect(execute.mock.calls[1]?.[0].change).toEqual(change);
  });

  it("accepts decimal numbers consistently and rejects an empty implicit set", async () => {
    const user = userEvent.setup();
    const execute = vi.fn(
      async (_command: DataSourceCellCommand<Row>) =>
        ({ status: "accepted" as const }) as DataSourceActionResult<Row>,
    );
    const numberField: DataSourceField<Row> = {
      id: "amount",
      label: "Amount",
      kind: "number",
      value: (candidate) => candidate.amount,
      sortable: true,
      filterable: false,
      groupable: false,
      canSet: () => true,
      canClear: () => true,
    };
    render(
      <DataViewCellEditor
        source={source(execute)}
        field={numberField}
        row={row}
        saveLabel="Save"
        clearLabel="Clear"
      />,
    );
    const input = screen.getByLabelText("Amount");
    expect(input).toHaveAttribute("step", "any");

    await user.clear(input);
    await user.type(input, "-1.5{Enter}");
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]?.[0].change).toEqual({
      op: "set",
      value: -1.5,
    });

    await user.clear(input);
    await user.type(input, "0");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(2));
    expect(execute.mock.calls[1]?.[0].change).toEqual({ op: "set", value: 0 });

    await user.clear(input);
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Enter a number or use Clear",
    );
    expect(execute).toHaveBeenCalledTimes(2);

    await user.click(screen.getByRole("button", { name: "Clear" }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(3));
    expect(execute.mock.calls[2]?.[0].change).toEqual({ op: "clear" });
  });
});
