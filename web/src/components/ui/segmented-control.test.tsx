// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { SegmentedControl } from "./segmented-control";

afterEach(cleanup);

test("single selection reports changes and cannot be cleared", () => {
  const onValueChange = vi.fn();
  const props = {
    "aria-label": "Example",
    onValueChange,
    options: [
      { value: "a", label: "First" },
      { value: "b", label: "Second" },
    ],
  };
  const { rerender } = render(<SegmentedControl {...props} value="a" />);
  fireEvent.click(screen.getByRole("radio", { name: "Second" }));
  expect(onValueChange).toHaveBeenCalledWith("b");
  rerender(<SegmentedControl {...props} value="b" />);
  expect(
    screen.getByRole("radio", { name: "Second" }).getAttribute("aria-checked"),
  ).toBe("true");
  onValueChange.mockClear();
  fireEvent.click(screen.getByRole("radio", { name: "Second" }));
  expect(onValueChange).not.toHaveBeenCalled();
});
