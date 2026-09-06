import { useEffect, useState, type ReactNode } from "react";
import { Badge, Box, Button, createTheme, Group, MantineProvider, Select, type CSSVariablesResolver } from "@mantine/core";

import { Icon } from "../icons";
import { readThemePreference, THEME_PREFERENCE_CHANGE_EVENT } from "./theme";

const theme = createTheme({
  respectReducedMotion: true,
  autoContrast: true,
  primaryShade: 8,
  // Black and white have equal WCAG contrast at this relative luminance.
  luminanceThreshold: 0.179,
  components: {
    Badge: Badge.extend({
      defaultProps: {
        tt: "none",
        h: "auto",
        styles: { label: { whiteSpace: "normal" } }
      }
    }),
    Button: Button.extend({ defaultProps: { size: "sm" } }),
    Select: Select.extend({
      defaultProps: {
        size: "sm",
        allowDeselect: false,
        withCheckIcon: false,
        rightSection: <Icon name="chevronDown" />,
        renderOption: ({ option, checked }) => (
          <Group gap="xs" wrap="nowrap">
            <Box w="1em">{checked ? <Icon name="circleCheck" /> : null}</Box>
            <span>{option.label}</span>
          </Group>
        ),
        comboboxProps: { transitionProps: { duration: 0 } }
      }
    })
  }
});

const readableTextVariables: CSSVariablesResolver = ({ colors }) => ({
  variables: {},
  light: { "--mantine-color-dimmed": colors.gray[7] },
  dark: { "--mantine-color-dimmed": colors.dark[1] }
});

function resolvedColorScheme(): "light" | "dark" {
  const preference = readThemePreference();
  return preference === "system"
    ? window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
    : preference;
}

export function ConsoleProvider({ children }: Readonly<{ children: ReactNode }>) {
  const [colorScheme, setColorScheme] = useState(resolvedColorScheme);

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const synchronize = () => setColorScheme(resolvedColorScheme());
    media.addEventListener("change", synchronize);
    window.addEventListener(THEME_PREFERENCE_CHANGE_EVENT, synchronize);
    synchronize();
    return () => {
      media.removeEventListener("change", synchronize);
      window.removeEventListener(THEME_PREFERENCE_CHANGE_EVENT, synchronize);
    };
  }, []);

  return (
    <MantineProvider theme={theme} forceColorScheme={colorScheme} cssVariablesResolver={readableTextVariables}>
      {children}
    </MantineProvider>
  );
}
