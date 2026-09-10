import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Icon } from "../../icons";
import type { MessageRule } from "./api";

type RuleBusy =
  | Readonly<{ kind: "item"; id: string }>
  | Readonly<{ kind: "collection"; collection: string }>
  | null;

type RuleFeedback = Readonly<{
  tone: "ok" | "error";
  content: ReactNode;
  reloadable: boolean;
}>;

type RulesPanelProps = Readonly<{
  items: readonly MessageRule[];
  busy: RuleBusy;
  feedback: RuleFeedback | null;
  onReload: () => void;
  onToggle: (rule: MessageRule) => void;
  onMove: (rule: MessageRule, direction: "up" | "down") => void;
}>;

type RuleCollection = Readonly<{
  name: string;
  items: readonly MessageRule[];
}>;

const knownCollectionMessageKeys: Readonly<Record<string, string>> = {
  auto_reply: "messages.rules.collections.autoReply"
};

function groupedCollections(items: readonly MessageRule[]): readonly RuleCollection[] {
  const collections = new Map<string, MessageRule[]>();
  for (const item of items) {
    const existing = collections.get(item.collection);
    if (existing) {
      existing.push(item);
    } else {
      collections.set(item.collection, [item]);
    }
  }

  return [...collections].map(([name, collectionItems]) => ({ name, items: collectionItems }));
}

function RuleCollectionView({
  collection,
  collectionIndex,
  busy,
  onToggle,
  onMove
}: Readonly<{
  collection: RuleCollection;
  collectionIndex: number;
  busy: RuleBusy;
  onToggle: (rule: MessageRule) => void;
  onMove: (rule: MessageRule, direction: "up" | "down") => void;
}>) {
  const { t } = useTranslation();
  const knownLabel = knownCollectionMessageKeys[collection.name];
  const collectionTitle = knownLabel
    ? t(knownLabel)
    : t("messages.rules.collections.unknown", { collection: collection.name });
  const collectionBusy = busy?.kind === "collection" && busy.collection === collection.name;
  const pageBusy = busy !== null;
  const headingID = `messages-rule-collection-${collectionIndex}`;

  return (
    <section data-messages-rule-collection aria-labelledby={headingID}>
      <div data-messages-rule-collection-heading>
        <div>
          <h3 id={headingID}>{collectionTitle}</h3>
          <p>{t("messages.rules.collectionDescription")}</p>
        </div>
        <span data-slot="badge" data-status={collectionBusy ? "pending" : "neutral"}>
          {t(
            collectionBusy
              ? "messages.rules.collectionSaving"
              : "messages.rules.collectionCount",
            { count: collection.items.length }
          )}
        </span>
      </div>
      <ol data-messages-rule-list>
        {collection.items.map((rule, itemIndex) => {
          const ruleTitleID = `messages-rule-${collectionIndex}-${itemIndex}-title`;
          const ruleDescriptionID = `messages-rule-${collectionIndex}-${itemIndex}-description`;
          const itemBusy = busy?.kind === "item" && busy.id === rule.id;
          const canMoveUp = itemIndex > 0;
          const canMoveDown = itemIndex < collection.items.length - 1;
          const definition = JSON.stringify(rule.definition, null, 2) ?? JSON.stringify(null);

          return (
            <li key={rule.id} data-messages-rule-item>
              <div data-slot="setting" data-rule-enabled={rule.enabled || undefined}>
                <div data-messages-rule-copy>
                  <div data-messages-rule-heading>
                    <h4 id={ruleTitleID}>{t("messages.rules.itemTitle", { number: itemIndex + 1 })}</h4>
                    <span data-slot="badge" data-status={rule.enabled ? "ok" : "neutral"}>
                      {t(rule.enabled ? "messages.rules.enabled" : "messages.rules.disabled")}
                    </span>
                  </div>
                  <p id={ruleDescriptionID}>{t("messages.rules.identifier", { id: rule.id })}</p>
                  <pre data-messages-rule-definition>{definition}</pre>
                </div>
                <div data-messages-rule-controls>
                  <button
                    type="button"
                    role="switch"
                    data-slot="switch"
                    aria-checked={rule.enabled}
                    aria-disabled={pageBusy ? "true" : undefined}
                    aria-labelledby={ruleTitleID}
                    aria-describedby={ruleDescriptionID}
                    onClick={() => {
                      if (!pageBusy) {
                        onToggle(rule);
                      }
                    }}
                  />
                  <div data-slot="button-group" data-size="sm" aria-label={t("messages.rules.orderActions")}>
                    <button
                      type="button"
                      data-slot="button"
                      data-variant="ghost"
                      data-size="sm"
                      aria-label={t("messages.rules.moveUp", { id: rule.id })}
                      aria-disabled={pageBusy || !canMoveUp ? "true" : undefined}
                      onClick={() => {
                        if (!pageBusy && canMoveUp) {
                          onMove(rule, "up");
                        }
                      }}
                    >
                      <Icon name="chevronUp" />
                      {t("messages.rules.up")}
                    </button>
                    <button
                      type="button"
                      data-slot="button"
                      data-variant="ghost"
                      data-size="sm"
                      aria-label={t("messages.rules.moveDown", { id: rule.id })}
                      aria-disabled={pageBusy || !canMoveDown ? "true" : undefined}
                      onClick={() => {
                        if (!pageBusy && canMoveDown) {
                          onMove(rule, "down");
                        }
                      }}
                    >
                      <Icon name="chevronDown" />
                      {t("messages.rules.down")}
                    </button>
                  </div>
                  {itemBusy ? (
                    <span data-slot="badge" data-status="pending">
                      {t("messages.rules.itemSaving")}
                    </span>
                  ) : null}
                </div>
              </div>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

export function RulesPanel({ items, busy, feedback, onReload, onToggle, onMove }: RulesPanelProps) {
  const { t } = useTranslation();
  const collections = groupedCollections(items);

  return (
    <section data-slot="card" data-messages-rules-section aria-labelledby="messages-rules-title">
      <div data-messages-section-heading>
        <h2 id="messages-rules-title">{t("messages.rules.title")}</h2>
        <p>{t("messages.rules.description")}</p>
      </div>
      <p data-messages-rules-semantics>{t("messages.rules.saveSemantics")}</p>
      {collections.length === 0 ? (
        <div data-messages-rules-empty>
          <h3 data-state-heading>
            <Icon name="circleMinus" />
            {t("messages.rules.empty.title")}
          </h3>
          <p>{t("messages.rules.empty.description")}</p>
        </div>
      ) : (
        <div data-messages-rule-collections>
          {collections.map((collection, collectionIndex) => (
            <RuleCollectionView
              key={collection.name}
              collection={collection}
              collectionIndex={collectionIndex}
              busy={busy}
              onToggle={onToggle}
              onMove={onMove}
            />
          ))}
        </div>
      )}
      {feedback ? (
        <div
          data-messages-rules-feedback
          data-tone={feedback.tone}
          role={feedback.tone === "error" ? "alert" : "status"}
        >
          <Icon name={feedback.tone === "ok" ? "circleCheck" : "circleAlert"} />
          {feedback.content}
          {feedback.reloadable ? (
            <button
              type="button"
              data-slot="button"
              data-variant="outline"
              data-size="sm"
              onClick={onReload}
            >
              <Icon name="refreshCw" />
              {t("messages.actions.reload")}
            </button>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
