import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";
import { Icon, type IconName } from "../icons";

export type AppSelectOption<Value extends string> = Readonly<{
  label: string;
  value: Value;
}>;

type AppSelectProps<Value extends string> = Readonly<{
  "aria-busy"?: boolean;
  "aria-disabled"?: boolean | "true" | "false";
  "aria-describedby"?: string;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  disabled?: boolean;
  /** Leading glyph for the current value. Callers that have no glyph omit it,
      and the trigger lays out the same either way. */
  icon?: IconName;
  id?: string;
  onValueChange: (value: Value) => void;
  options: readonly AppSelectOption<Value>[];
  value: Value;
}>;

function selectedOptionIndex<Value extends string>(
  options: readonly AppSelectOption<Value>[],
  value: Value
): number {
  const index = options.findIndex((option) => option.value === value);
  return index < 0 ? 0 : index;
}

export function AppSelect<Value extends string>({
  "aria-busy": ariaBusy,
  "aria-disabled": ariaDisabled,
  "aria-describedby": ariaDescribedBy,
  "aria-label": ariaLabel,
  "aria-labelledby": ariaLabelledBy,
  disabled = false,
  icon,
  id,
  onValueChange,
  options,
  value
}: AppSelectProps<Value>) {
  const generatedListboxId = useId();
  const listboxId = `${id ?? generatedListboxId}-options`;
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const optionRefs = useRef(new Map<number, HTMLDivElement>());
  const selectedIndex = selectedOptionIndex(options, value);
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(selectedIndex);
  const selectedOption = options[selectedIndex];
  const ariaDisabledState = ariaDisabled === true || ariaDisabled === "true";

  useEffect(() => {
    if (!open) {
      setActiveIndex(selectedIndex);
    }
  }, [open, selectedIndex]);

  useEffect(() => {
    if (ariaDisabledState && open) {
      setOpen(false);
      triggerRef.current?.focus();
    }
  }, [ariaDisabledState, open]);

  useEffect(() => {
    if (!open) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      optionRefs.current.get(activeIndex)?.focus();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [activeIndex, open]);

  useEffect(() => {
    if (!open) {
      return;
    }

    function closeOutside(event: PointerEvent): void {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    }

    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, [open]);

  function openListbox(index = selectedIndex): void {
    if (disabled || ariaDisabledState || options.length === 0) {
      return;
    }

    setActiveIndex(index);
    setOpen(true);
  }

  function closeListbox(returnFocus = false): void {
    setOpen(false);
    if (returnFocus) {
      window.requestAnimationFrame(() => triggerRef.current?.focus());
    }
  }

  function choose(index: number): void {
    if (ariaDisabledState) {
      return;
    }

    const option = options[index];
    if (!option) {
      return;
    }

    onValueChange(option.value);
    closeListbox(true);
  }

  function moveActive(step: number): void {
    if (options.length === 0) {
      return;
    }

    setActiveIndex((current) => (current + step + options.length) % options.length);
  }

  function handleTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>): void {
    if (disabled || ariaDisabledState || options.length === 0) {
      return;
    }

    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        openListbox(Math.min(selectedIndex + 1, options.length - 1));
        break;
      case "ArrowUp":
        event.preventDefault();
        openListbox(Math.max(selectedIndex - 1, 0));
        break;
      case "Home":
        event.preventDefault();
        openListbox(0);
        break;
      case "End":
        event.preventDefault();
        openListbox(options.length - 1);
        break;
      case "Enter":
      case " ":
        event.preventDefault();
        if (open) {
          closeListbox();
        } else {
          openListbox();
        }
        break;
      case "Escape":
        if (open) {
          event.preventDefault();
          closeListbox(true);
        }
        break;
      default:
        break;
    }
  }

  function handleOptionKeyDown(
    event: KeyboardEvent<HTMLDivElement>,
    index: number
  ): void {
    if (ariaDisabledState) {
      return;
    }

    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        moveActive(1);
        break;
      case "ArrowUp":
        event.preventDefault();
        moveActive(-1);
        break;
      case "Home":
        event.preventDefault();
        setActiveIndex(0);
        break;
      case "End":
        event.preventDefault();
        setActiveIndex(options.length - 1);
        break;
      case "Enter":
      case " ":
        event.preventDefault();
        choose(index);
        break;
      case "Escape":
        event.preventDefault();
        closeListbox(true);
        break;
      case "Tab":
        closeListbox();
        break;
      default:
        break;
    }
  }

  return (
    <div ref={rootRef} data-slot="select">
      <button
        ref={triggerRef}
        aria-busy={ariaBusy}
        aria-disabled={ariaDisabled}
        aria-controls={listboxId}
        aria-describedby={ariaDescribedBy}
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-label={ariaLabel}
        aria-labelledby={ariaLabelledBy}
        data-slot="select-trigger"
        data-value={value}
        disabled={disabled || options.length === 0}
        id={id}
        type="button"
        onClick={() => {
          if (!ariaDisabledState) {
            open ? closeListbox() : openListbox();
          }
        }}
        onKeyDown={handleTriggerKeyDown}
      >
        {icon ? <Icon name={icon} /> : null}
        <span data-slot="select-value">{selectedOption?.label}</span>
      </button>
      {open ? (
        <div
          aria-label={ariaLabel}
          aria-labelledby={ariaLabelledBy}
          data-slot="select-content"
          id={listboxId}
          role="listbox"
        >
          {options.map((option, index) => (
            <div
              key={option.value}
              ref={(element) => {
                if (element) {
                  optionRefs.current.set(index, element);
                } else {
                  optionRefs.current.delete(index);
                }
              }}
              aria-selected={option.value === value}
              data-slot="option"
              data-value={option.value}
              role="option"
              tabIndex={index === activeIndex ? 0 : -1}
              onClick={() => choose(index)}
              onKeyDown={(event) => handleOptionKeyDown(event, index)}
            >
              {option.label}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}
