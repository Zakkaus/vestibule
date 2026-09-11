import { style } from "@react-spectrum/s2/style" with { type: "macro" };

// A section is a heading and the rows under it, on the page's own ground. The library's
// documentation and the Adobe product built on it both put content on one surface and
// tint at most one level inside it; wrapping each section in a tile as well left the page
// a stack of boxes inside boxes with nothing to look at first.
export const sectionSurface = style({
  display: "grid",
  alignContent: "start",
  gap: { default: 12, "@media (max-height: 800px)": 8 },
  width: "full",
  minWidth: 0,
  boxSizing: "border-box"
});
