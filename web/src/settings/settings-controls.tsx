import { cloneElement, isValidElement, useId, type ReactNode } from "react";
import { Label } from "../components/ui/label";
import { Switch } from "../components/ui/switch";

export function Field({
  label,
  children,
  help,
  inputId,
}: {
  label: string;
  children: ReactNode;
  help?: string;
  inputId?: string;
}) {
  const generatedId = useId();
  const id = inputId || generatedId;
  const control =
    !inputId &&
    isValidElement<{ id?: string; "aria-describedby"?: string }>(children)
      ? cloneElement(children, {
          id,
          "aria-describedby": help ? `${id}-help` : undefined,
        })
      : children;
  return (
    <div className="field">
      <Label htmlFor={id}>{label}</Label>
      <div>
        {control}
        {help && <small id={`${id}-help`}>{help}</small>}
      </div>
    </div>
  );
}

export function Toggle({
  label,
  value,
  onChange,
  disabled = false,
  id,
  "aria-describedby": describedBy,
}: {
  label: string;
  value: boolean;
  onChange: (value: boolean) => void;
  disabled?: boolean;
  id?: string;
  "aria-describedby"?: string;
}) {
  const generatedId = useId();
  const controlId = id || generatedId;
  return (
    <div className="toggle">
      <Switch
        id={controlId}
        aria-label={label}
        aria-describedby={describedBy}
        checked={value}
        disabled={disabled}
        onCheckedChange={onChange}
      />
      <Label htmlFor={controlId}>{label}</Label>
    </div>
  );
}
