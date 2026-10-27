import { cn } from "../../lib/utils";
import { ToggleGroup, ToggleGroupItem } from "./toggle-group";

type SegmentedControlProps = {
  "aria-label": string;
  id?: string;
  className?: string;
  value: string;
  onValueChange: (value: string) => void;
  options: readonly { value: string; label: string }[];
};

export function SegmentedControl({
  options,
  className,
  onValueChange,
  value,
  ...props
}: SegmentedControlProps) {
  return (
    <ToggleGroup
      {...props}
      value={value}
      type="single"
      size="sm"
      className={cn(
        "flex flex-wrap justify-start gap-1 rounded-lg border border-border bg-muted p-1",
        className,
      )}
      onValueChange={(value) => {
        if (value) onValueChange(value);
      }}
    >
      {options.map((option) => (
        <ToggleGroupItem
          key={option.value}
          value={option.value}
          className="min-w-fit flex-1 border border-transparent px-3 text-muted-foreground duration-100 ease-out hover:bg-background hover:text-foreground data-[state=on]:border-border data-[state=on]:bg-background data-[state=on]:text-foreground motion-reduce:transition-none"
        >
          {option.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
