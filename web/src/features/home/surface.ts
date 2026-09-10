import { style } from "@react-spectrum/s2/style" with { type: "macro" };

// The tokens Card draws its surface with, for sections that are not cards. Card lays its
// children out by slot — title, description, footer — and given a heading followed by a row
// of metrics it put the heading in the title area and the metrics at the bottom. These
// sections want the surface and nothing else.
export const sectionSurface = style({
  display: "grid",
  alignContent: "start",
  gap: 16,
  width: "full",
  minWidth: 0,
  boxSizing: "border-box",
  padding: 24,
  borderRadius: "lg",
  backgroundColor: "layer-1"
});
