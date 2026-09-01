import type {
  BypassSettings,
  BypassSettingsChanges,
  SettingSource,
  SettingValue
} from "./api";

export type BypassField =
  | "trustedMemberGroupIDs"
  | "requiredChannelID"
  | "requiredChannelFailOpen"
  | "channelDisplay"
  | "channelInviteURL"
  | "channelWhitelist";

export type BypassTextField = Exclude<BypassField, "requiredChannelFailOpen">;

export type BypassForm = Readonly<{
  trustedMemberGroupIDs: string;
  requiredChannelID: string;
  requiredChannelFailOpen: boolean;
  channelDisplay: string;
  channelInviteURL: string;
  channelWhitelist: string;
}>;

export type RestoringFields = Readonly<Partial<Record<BypassField, true>>>;

export type BypassValidationError = "integer" | "list" | "reachableChannel";

export type BypassValidationErrors = Readonly<
  Partial<Record<BypassField, BypassValidationError>>
>;

export type BypassEvaluation = Readonly<{
  changes: BypassSettingsChanges;
  count: number;
  errors: BypassValidationErrors;
  valid: boolean;
}>;

export function formFromSettings(settings: BypassSettings): BypassForm {
  return {
    trustedMemberGroupIDs: settings.trustedMemberGroupIDs.value.join("\n"),
    requiredChannelID:
      settings.requiredChannelID.value === 0 ? "" : String(settings.requiredChannelID.value),
    requiredChannelFailOpen: settings.requiredChannelFailOpen.value,
    channelDisplay: settings.channelDisplay.value,
    channelInviteURL: settings.channelInviteURL.value,
    channelWhitelist: settings.channelWhitelist.value.join("\n")
  };
}

export function settingForField(
  settings: BypassSettings,
  field: BypassField
): SettingValue<readonly number[] | number | boolean | string> {
  switch (field) {
    case "trustedMemberGroupIDs":
      return settings.trustedMemberGroupIDs;
    case "requiredChannelID":
      return settings.requiredChannelID;
    case "requiredChannelFailOpen":
      return settings.requiredChannelFailOpen;
    case "channelDisplay":
      return settings.channelDisplay;
    case "channelInviteURL":
      return settings.channelInviteURL;
    case "channelWhitelist":
      return settings.channelWhitelist;
  }
}

export function sourceForField(settings: BypassSettings, field: BypassField): SettingSource {
  return settingForField(settings, field).source;
}

function parsedInteger(raw: string): number | undefined {
  const value = raw.trim();
  if (!/^-?\d+$/.test(value)) {
    return undefined;
  }

  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : undefined;
}

function parsedIDList(raw: string): readonly number[] | undefined {
  const entries = raw
    .split("\n")
    .map((entry) => entry.trim())
    .filter((entry) => entry !== "");
  const ids: number[] = [];
  const seen = new Set<number>();

  for (const entry of entries) {
    const id = parsedInteger(entry);
    if (id === undefined || id === 0 || seen.has(id)) {
      return undefined;
    }
    seen.add(id);
    ids.push(id);
  }

  return ids;
}

function sameIDs(left: readonly number[], right: readonly number[]): boolean {
  if (left.length !== right.length) {
    return false;
  }

  for (let index = 0; index < left.length; index += 1) {
    if (left[index] !== right[index]) {
      return false;
    }
  }

  return true;
}

export function evaluateBypassForm(
  settings: BypassSettings,
  form: BypassForm,
  restoring: RestoringFields
): BypassEvaluation {
  const changes: {
    trusted_member_group_ids?: readonly number[] | null;
    required_channel_id?: number | null;
    required_channel_fail_open?: boolean | null;
    channel_display?: string | null;
    channel_invite_url?: string | null;
    channel_whitelist?: readonly number[] | null;
  } = {};
  const errors: Partial<Record<BypassField, BypassValidationError>> = {};

  if (restoring.trustedMemberGroupIDs) {
    changes.trusted_member_group_ids = null;
  } else {
    const trustedMemberGroupIDs = parsedIDList(form.trustedMemberGroupIDs);
    if (trustedMemberGroupIDs === undefined) {
      errors.trustedMemberGroupIDs = "list";
    } else if (!sameIDs(trustedMemberGroupIDs, settings.trustedMemberGroupIDs.value)) {
      changes.trusted_member_group_ids = trustedMemberGroupIDs;
    }
  }

  let requiredChannelID = settings.requiredChannelID.value;
  if (restoring.requiredChannelID) {
    changes.required_channel_id = null;
  } else {
    const parsedRequiredChannelID =
      form.requiredChannelID.trim() === "" ? 0 : parsedInteger(form.requiredChannelID);
    if (parsedRequiredChannelID === undefined) {
      errors.requiredChannelID = "integer";
    } else {
      requiredChannelID = parsedRequiredChannelID;
      if (requiredChannelID !== settings.requiredChannelID.value) {
        changes.required_channel_id = requiredChannelID;
      }
    }
  }

  if (restoring.requiredChannelFailOpen) {
    changes.required_channel_fail_open = null;
  } else if (form.requiredChannelFailOpen !== settings.requiredChannelFailOpen.value) {
    changes.required_channel_fail_open = form.requiredChannelFailOpen;
  }

  let channelDisplay = settings.channelDisplay.value;
  if (restoring.channelDisplay) {
    changes.channel_display = null;
  } else {
    channelDisplay = form.channelDisplay.trim();
    if (channelDisplay !== settings.channelDisplay.value) {
      changes.channel_display = channelDisplay;
    }
  }

  let channelInviteURL = settings.channelInviteURL.value;
  if (restoring.channelInviteURL) {
    changes.channel_invite_url = null;
  } else {
    channelInviteURL = form.channelInviteURL.trim();
    if (channelInviteURL !== settings.channelInviteURL.value) {
      changes.channel_invite_url = channelInviteURL;
    }
  }

  if (restoring.channelWhitelist) {
    changes.channel_whitelist = null;
  } else {
    const channelWhitelist = parsedIDList(form.channelWhitelist);
    if (channelWhitelist === undefined) {
      errors.channelWhitelist = "list";
    } else if (!sameIDs(channelWhitelist, settings.channelWhitelist.value)) {
      changes.channel_whitelist = channelWhitelist;
    }
  }

  if (
    errors.requiredChannelID === undefined &&
    !restoring.requiredChannelID &&
    !restoring.channelDisplay &&
    !restoring.channelInviteURL &&
    requiredChannelID !== 0 &&
    channelInviteURL === "" &&
    !channelDisplay.startsWith("@")
  ) {
    errors.requiredChannelID = "reachableChannel";
  }

  const count = Object.keys(changes).length + Object.keys(errors).length;
  return {
    changes,
    count,
    errors,
    valid: Object.keys(errors).length === 0
  };
}
