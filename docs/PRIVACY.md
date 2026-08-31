**English** · [简体中文](PRIVACY.zh-CN.md)

# What this bot holds about people

**This software stores data about members of groups it does not own.** Anyone who
runs a copy of it is holding records about people who never agreed anything with
them directly — they joined a Telegram group, and the group's administrators
pointed this bot at it.

That is the first thing to know, so it is the first thing written here.

There is **no single official instance**. This is an open-source project; anyone
can run it. Whoever runs the copy you are dealing with is the one holding your
data, deciding how long to keep it, and answerable for it. This document
describes what *the software* does, which is the same everywhere. What *an
instance* does is up to its operator, and section 4 is where they say so.

## 1. What it reads

- **Join requests.** When someone asks to join a group under approval mode,
  Telegram sends the bot that request, with the applicant's user id, username and
  display name.
- **Membership events.** People joining, leaving, being promoted or restricted.
- **Messages in groups where it is an administrator.** Telegram's privacy mode is
  off for administrators, so the bot receives every message posted in those
  groups. It uses them for the anti-spam and auto-reply features and does not
  store message text.
- **Direct messages to the bot**, which is how an applicant answers a challenge.

## 2. What it stores

Every table below is in `migrations/00-latest.sql`, and this list is checked
against it — `scripts/check-privacy-tables.py` fails if the schema grows a table
holding a user or chat identifier and this document does not name it.

| Table | About whom | What it holds |
|---|---|---|
| `chat` | a group, not a person | the group's id and title, and its settings |
| `challenge` | an applicant | user id, group id, the state it ended in and why, attempts, timestamps, which administrator settled it, and a payload containing the question asked, the options, the applicant's **display name at the time**, and the message ids the challenge was delivered as |
| `verification_failure` | an applicant | user id, group id, how many times they failed, when last |
| `warning_counter` | a member | user id, group id, how many warnings they hold |
| `rule` | nobody directly | a group's questions, replies and filters — which can name people if an administrator writes them that way |

Two tables hold no personal data: `agent_tally` counts self-reported model names
from the challenge's tripwire, and `verification_runtime` holds two numbers.

**It does not store:** message text, phone numbers, email addresses, IP
addresses, location, or anything Telegram did not send with the events above. It
does not read messages in groups where it is not an administrator, and it cannot
read them in Telegram's secret chats at all.

## 3. How long

- A **challenge** survives its settlement, because the operations log is how an
  administrator sees what happened and undoes a mistake.
- **Failure counts** are what the cooldown is made of, and expire with it.
- **Warnings** last until they are cleared or the counter is bounded out.
- When the bot is removed from a group, or an administrator deletes the group's
  data from the console, that group's records are erased.

An instance may keep less than this. It cannot keep more without changing the
code.

## 4. What the operator decides, and must state

Copy this section into your own instance's page and fill it in. Leaving it blank
is itself an answer, and not a good one.

- **Who runs this instance**, and how to reach them.
- **Where the database lives** — which machine, which country.
- **Who can read the console**, and therefore who can see the records above.
- **How long logs are kept.** Logs are separate from the database and are
  described in section 5.
- **Whether anything is shared** with anyone else. The software shares nothing;
  an operator can.
- **How someone asks for their records to be removed**, and how long that takes.

## 5. Logs

The bot writes an operations log. It carries **user ids and usernames** — a line
reads `join 8913270020 (@someone) in group -100…` — along with what was decided
and why. It does not carry message text or challenge answers.

Where those logs go, who can read them, and how long they are kept are the
operator's to decide and to state in section 4. On a systemd installation they
go to the journal and follow that machine's retention.

## 6. If you are a group member

You are dealing with two parties, and they are usually not the same people:

- **Your group's administrators** chose to use this bot, chose its questions, and
  can see and undo its decisions for your group.
- **The operator** runs the instance and holds the database.

Ask your group's administrators first — they can delete your records for their
group from the console. If you need more than that, they can tell you who the
operator is.

## 7. If you run an instance

You are the one holding this data. The software gives you the tools to keep that
honest:

- Deleting a group's data from the console erases it, and the console tells the
  administrator how long that takes.
- Removing the bot from a group has the same effect.
- Everything in section 2 is inspectable — it is a plain SQL schema and you can
  read it.

Nothing here is legal advice, and this document does not make your instance
compliant with anything. It tells you exactly what you are holding, which is
where any honest answer has to start.
