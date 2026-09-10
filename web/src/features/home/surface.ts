import { style } from "@react-spectrum/s2/style" with { type: "macro" };

// The tokens Card draws its surface with, for sections that are not cards. Card lays its
// children out by slot — title, description, footer — and given a heading followed by a row
// of metrics it put the heading in the title area and the metrics at the bottom. These
// sections want the surface and nothing else.
export const sectionSurface = style({
  display: "grid",
  alignContent: "start",
  // A short window has to hold the whole page; the ladder steps down rather than scrolling.
  gap: { default: 12, "@media (max-height: 800px)": 8 },
  width: "full",
  minWidth: 0,
  boxSizing: "border-box",
  padding: { default: 20, "@media (max-height: 800px)": 16 },
  borderRadius: "lg",
  backgroundColor: "layer-1"
});
