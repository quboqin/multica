import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CollectionField } from "@multica/core/collections";
import { renderWithI18n } from "../test/i18n";
import { CollectionFieldPanel } from "./collection-field-panel";
import type { CollectionCommands } from "./use-collection-commands";

const TODO = "11111111-1111-4111-8111-111111111111";
const DONE = "22222222-2222-4222-8222-222222222222";
const stage: CollectionField = {
  id: "stage",
  name: "Stage",
  type: "select",
  position: 0,
  config: {
    options: [
      { id: TODO, name: "Todo", color: "#dc2626" },
      { id: DONE, name: "Done", color: "#2563eb" },
    ],
  },
};

function commandsWith(
  overrides: Partial<
    Record<"createField" | "updateField" | "renameTitleColumn", unknown>
  > = {},
) {
  return {
    createField: { mutateAsync: vi.fn().mockResolvedValue({}), isPending: false },
    updateField: { mutateAsync: vi.fn().mockResolvedValue({}), isPending: false },
    renameTitleColumn: { mutateAsync: vi.fn().mockResolvedValue({}), isPending: false },
    ...overrides,
  } as unknown as CollectionCommands;
}

afterEach(cleanup);

describe("field panel", () => {
  it("creates a select field with its options from one panel", async () => {
    const user = userEvent.setup();
    const commands = commandsWith();
    const onOpenChange = vi.fn();
    renderWithI18n(
      <CollectionFieldPanel open anchor={null} target={{ kind: "new" }} fieldCount={0} commands={commands} onOpenChange={onOpenChange} />,
    );
    expect(screen.getByRole("dialog", { name: "New field" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Name"), " Stage ");
    await user.click(screen.getByRole("combobox", { name: "Field type" }));
    await user.click(await screen.findByRole("option", { name: "Select" }));
    await user.type(await screen.findByLabelText("Option 1"), "Todo");
    await user.click(screen.getByRole("button", { name: "Create field" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(commands.createField.mutateAsync).toHaveBeenCalledWith({
      name: "Stage",
      type: "select",
      config: { options: [{ name: "Todo", color: expect.any(String) }] },
    });
  });

  it("offers an existing field only the conversions that keep its values", async () => {
    renderWithI18n(
      <CollectionFieldPanel open anchor={null} target={{ kind: "field", field: stage }} fieldCount={1} commands={commandsWith()} onOpenChange={vi.fn()} />,
    );
    expect(screen.getByRole("dialog", { name: "Edit field" })).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toHaveValue("Stage");
    await userEvent.setup().click(screen.getByRole("combobox", { name: "Field type" }));
    const options = await screen.findAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual(["Select", "Multi-select"]);
  });

  it("stops at the table's field quota before asking the server", () => {
    const commands = commandsWith();
    renderWithI18n(
      <CollectionFieldPanel open anchor={null} target={{ kind: "new" }} fieldCount={50} commands={commands} onOpenChange={vi.fn()} />,
    );
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "One more" } });
    expect(screen.getByRole("button", { name: "Create field" })).toBeDisabled();
    expect(screen.getByText("50 / 50 fields")).toBeInTheDocument();
  });

  it("renames, removes and adds select options in one save", async () => {
    const commands = commandsWith();
    const onOpenChange = vi.fn();
    renderWithI18n(
      <CollectionFieldPanel open anchor={null} target={{ kind: "field", field: stage }} fieldCount={1} commands={commands} onOpenChange={onOpenChange} />,
    );
    fireEvent.change(screen.getByLabelText("Option 1"), { target: { value: "Backlog" } });
    fireEvent.click(screen.getByRole("button", { name: "Remove Done" }));
    expect(screen.getByText(/Saving clears Done/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add option" }));
    fireEvent.change(screen.getByLabelText("Option 2"), { target: { value: "Shipped" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(commands.updateField.mutateAsync).toHaveBeenCalledWith({
      fieldId: "stage",
      patch: {
        config: {
          options: [
            { id: TODO, name: "Backlog", color: "#dc2626" },
            { name: "Shipped", color: expect.any(String) },
          ],
        },
      },
    });
  });

  it("starts from the field type the caller is missing", async () => {
    const commands = commandsWith();
    renderWithI18n(
      <CollectionFieldPanel open anchor={null} target={{ kind: "new", type: "select" }} fieldCount={0} commands={commands} onOpenChange={vi.fn()} />,
    );
    expect(screen.getByRole("combobox", { name: "Field type" })).toHaveTextContent("Select");
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Stage" } });
    fireEvent.change(screen.getByLabelText("Option 1"), { target: { value: "Todo" } });
    fireEvent.click(screen.getByRole("button", { name: "Create field" }));
    await waitFor(() =>
      expect(commands.createField.mutateAsync).toHaveBeenCalledWith({
        name: "Stage",
        type: "select",
        config: { options: [{ name: "Todo", color: expect.any(String) }] },
      }),
    );
  });

  it("renames the title column without offering another type", async () => {
    const commands = commandsWith();
    const onOpenChange = vi.fn();
    renderWithI18n(
      <CollectionFieldPanel open anchor={null} target={{ kind: "title", name: "", defaultName: "Name" }} fieldCount={3} commands={commands} onOpenChange={onOpenChange} />,
    );
    expect(screen.getByRole("dialog", { name: "Edit field" })).toBeInTheDocument();
    const name = screen.getByLabelText("Name");
    // An unnamed title column shows its localized default, ready to edit.
    expect(name).toHaveValue("Name");
    expect(screen.getByRole("combobox", { name: "Field type" })).toBeDisabled();
    expect(screen.getByText(/each record's title/)).toBeInTheDocument();
    // The title column is outside the field quota.
    expect(screen.queryByText("3 / 50 fields")).not.toBeInTheDocument();
    fireEvent.change(name, { target: { value: " Customer " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(commands.renameTitleColumn.mutateAsync).toHaveBeenCalledWith("Customer");
    expect(commands.updateField.mutateAsync).not.toHaveBeenCalled();
  });

  it("returns the title column to its default when the label is cleared or typed back", async () => {
    for (const value of ["", "Name"]) {
      const commands = commandsWith();
      const { unmount } = renderWithI18n(
        <CollectionFieldPanel open anchor={null} target={{ kind: "title", name: "Customer", defaultName: "Name" }} fieldCount={0} commands={commands} onOpenChange={vi.fn()} />,
      );
      expect(screen.getByLabelText("Name")).toHaveAttribute("placeholder", "Name");
      fireEvent.change(screen.getByLabelText("Name"), { target: { value } });
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
      // The default is never stored, so it keeps following the language.
      await waitFor(() =>
        expect(commands.renameTitleColumn.mutateAsync).toHaveBeenCalledWith(""),
      );
      unmount();
    }
  });

  it("saves nothing when the title column label did not change", async () => {
    const commands = commandsWith();
    const onOpenChange = vi.fn();
    renderWithI18n(
      <CollectionFieldPanel open anchor={null} target={{ kind: "title", name: "", defaultName: "Name" }} fieldCount={0} commands={commands} onOpenChange={onOpenChange} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(commands.renameTitleColumn.mutateAsync).not.toHaveBeenCalled();
  });

  it("keeps the panel open with the server error when saving fails", async () => {
    const commands = commandsWith({
      createField: {
        mutateAsync: vi.fn().mockRejectedValue(new Error("a field with that name already exists")),
        isPending: false,
      },
    });
    const onOpenChange = vi.fn();
    renderWithI18n(
      <CollectionFieldPanel open anchor={null} target={{ kind: "new" }} fieldCount={0} commands={commands} onOpenChange={onOpenChange} />,
    );
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Status" } });
    fireEvent.click(screen.getByRole("button", { name: "Create field" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("already exists");
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});
