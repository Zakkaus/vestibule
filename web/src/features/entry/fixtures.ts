import type { IconName } from "../../icons";

export type EntryFixture = {
  id: string;
  /** The glyph for this state. Every fixture carries one: the two states that
      had a heading icon were the two that are not fixtures, so the screens a
      person actually lands on were the ones without. */
  icon: IconName;
  titleKey: string;
  descriptionKey: string;
  stepKeys: readonly string[];
  interpolation: Record<string, string>;
};

const defaultEntryFixture: EntryFixture = {
  id: "no-session",
  icon: "messagesSquare",
  titleKey: "entry.noSession.title",
  descriptionKey: "entry.noSession.description",
  stepKeys: [
    "entry.noSession.steps.openBot",
    "entry.noSession.steps.manager",
    "entry.noSession.steps.operator"
  ],
  interpolation: {}
};

// An instance nobody has claimed has no bot, so it cannot tell anyone to open
// one. Its way in is the link the install script printed, not Telegram.
export const unclaimedEntryFixture: EntryFixture = {
  id: "unclaimed",
  icon: "unlock",
  titleKey: "entry.unclaimed.title",
  descriptionKey: "entry.unclaimed.description",
  stepKeys: ["entry.unclaimed.steps.link", "entry.unclaimed.steps.token"],
  interpolation: {}
};

export const entryFixtures: readonly EntryFixture[] = [
  defaultEntryFixture,
  unclaimedEntryFixture,
  {
    id: "expired",
    icon: "timerOff",
    titleKey: "entry.expired.title",
    descriptionKey: "entry.expired.description",
    stepKeys: ["entry.expired.steps.request"],
    interpolation: {}
  },
  {
    id: "redeemed",
    icon: "link2Off",
    titleKey: "entry.redeemed.title",
    descriptionKey: "entry.redeemed.description",
    stepKeys: ["entry.redeemed.steps.request"],
    interpolation: {}
  },
  {
    id: "no-groups",
    icon: "usersRound",
    titleKey: "entry.noGroups.title",
    descriptionKey: "entry.noGroups.description",
    stepKeys: ["entry.noGroups.steps.authorization"],
    interpolation: { accountId: "741928306" }
  },
  {
    id: "outside-telegram",
    icon: "monitor",
    titleKey: "entry.outsideTelegram.title",
    descriptionKey: "entry.outsideTelegram.description",
    stepKeys: ["entry.outsideTelegram.steps.desktop"],
    interpolation: {}
  }
];

export function entryFixtureFor(stateId: string | null): EntryFixture {
  return entryFixtures.find((fixture) => fixture.id === stateId) ?? defaultEntryFixture;
}
