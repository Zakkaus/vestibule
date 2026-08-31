export type EntryFixture = {
  id: string;
  titleKey: string;
  descriptionKey: string;
  stepKeys: readonly string[];
  interpolation: Record<string, string>;
};

const botUsername = "@gentoo_zh_verify_bot";

const defaultEntryFixture: EntryFixture = {
  id: "no-session",
  titleKey: "entry.noSession.title",
  descriptionKey: "entry.noSession.description",
  stepKeys: [
    "entry.noSession.steps.openBot",
    "entry.noSession.steps.manager",
    "entry.noSession.steps.operator"
  ],
  interpolation: {
    botUsername
  }
};

export const entryFixtures: readonly EntryFixture[] = [
  defaultEntryFixture,
  {
    id: "expired",
    titleKey: "entry.expired.title",
    descriptionKey: "entry.expired.description",
    stepKeys: ["entry.expired.steps.request"],
    interpolation: {
      botUsername
    }
  },
  {
    id: "redeemed",
    titleKey: "entry.redeemed.title",
    descriptionKey: "entry.redeemed.description",
    stepKeys: ["entry.redeemed.steps.request"],
    interpolation: {
      botUsername
    }
  },
  {
    id: "no-groups",
    titleKey: "entry.noGroups.title",
    descriptionKey: "entry.noGroups.description",
    stepKeys: ["entry.noGroups.steps.authorization"],
    interpolation: {
      accountId: "741928306"
    }
  },
  {
    id: "outside-telegram",
    titleKey: "entry.outsideTelegram.title",
    descriptionKey: "entry.outsideTelegram.description",
    stepKeys: ["entry.outsideTelegram.steps.desktop"],
    interpolation: {
      botUsername
    }
  }
];

export function entryFixtureFor(stateId: string | null): EntryFixture {
  return entryFixtures.find((fixture) => fixture.id === stateId) ?? defaultEntryFixture;
}
