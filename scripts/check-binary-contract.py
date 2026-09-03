#!/usr/bin/env python3
"""The deployment and the binary agree on how to start, configure and stop the bot.

cmd/bot has no test of any kind, and everything it does is a promise to something outside the Go
code: the unit passes a flag by name, the unit and the compose file set environment variables by
name, systemd stops the service with a signal, and Type=notify means the process is expected to
answer on a socket it finds in the environment. Rename any of those on one side and the other side
keeps using the old name -- nothing in the repository compares them.

What each mismatch costs is different, so each is its own rule.
"""

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MAIN = ROOT / "cmd" / "bot" / "main.go"
UNIT = ROOT / "deploy" / "vestibule.service"
COMPOSE = ROOT / "deploy" / "compose.yaml"

# systemd's default when a unit sets no KillSignal.
DEFAULT_KILL_SIGNAL = "SIGTERM"


def read(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def main() -> int:
    for path in (MAIN, UNIT, COMPOSE):
        if not path.exists():
            print(f"FAIL check-binary-contract: {path} is missing")
            return 1
    main_go, unit, compose = read(MAIN), read(UNIT), read(COMPOSE)
    failures: list[str] = []

    flags = set(re.findall(r'flag\.(?:String|Bool|Int|Duration)\(\s*"([a-z-]+)"', main_go))
    if not flags:
        failures.append("no flag was read out of cmd/bot/main.go; the parser is wrong")

    exec_start = re.search(r"^ExecStart=(.*)$", unit, re.M)
    if exec_start is None:
        failures.append("the unit has no ExecStart line")
    else:
        # `env --` ends that tool's own arguments; the bare -- is not a flag of the binary.
        after_separator = exec_start.group(1).split(" -- ", 1)[-1]
        passed = {name for name in re.findall(r"(?:^|\s)--?([a-z][a-z-]*)", after_separator)}
        for name in sorted(passed):
            if name not in flags:
                failures.append(
                    f"the unit starts the binary with --{name} and cmd/bot defines no such flag; "
                    f"the service would exit before it ever read its configuration"
                )

    read_names = set(re.findall(r'os\.Getenv\("([A-Z_]+)"\)', main_go))
    if not read_names:
        failures.append("no environment name was read out of cmd/bot/main.go; the parser is wrong")

    # Only what is set for THIS process is compared. The compose file also runs a database and a
    # Bot API server, and their settings are theirs; reading every environment: block in the file
    # reported POSTGRES_PASSWORD as a name the bot ignores, which is true and not a defect.
    def bot_environment(text: str) -> set:
        names: set = set()
        in_app = in_env = False
        for line in text.split("\n"):
            if re.match(r"^  [A-Za-z0-9_-]+:\s*$", line):
                in_app = line.strip().rstrip(":") == "app"
                in_env = False
                continue
            if in_app and re.match(r"^    environment:\s*$", line):
                in_env = True
                continue
            if in_env:
                if not line.startswith("      "):
                    in_env = False
                    continue
                found = re.match(r"^      ([A-Z][A-Z0-9_]*):", line)
                if found:
                    names.add(found.group(1))
        return names

    set_names = {"the unit": set(re.findall(r"^Environment=([A-Z_]+)=", unit, re.M)),
                 "the compose file": bot_environment(compose)}
    if not set_names["the compose file"]:
        failures.append("no environment name was read out of the compose file's app service")
    for source, names in set_names.items():
        for name in sorted(names):
            if name in read_names:
                continue
            failures.append(
                f"{source} sets {name} for the bot and cmd/bot never reads it; the setting is inert "
                f"and the operator has no way to tell"
            )

    signals = set(re.findall(r"syscall\.(SIG[A-Z0-9]+)", main_go))
    kill_signal = re.search(r"^KillSignal=(\S+)$", unit, re.M)
    expected = kill_signal.group(1) if kill_signal else DEFAULT_KILL_SIGNAL
    if expected not in signals:
        failures.append(
            f"systemd stops this service with {expected} and cmd/bot installs a handler for "
            f"{sorted(signals) or 'nothing'}; the process would be killed outright at "
            f"TimeoutStopSec instead of shutting down, and a verification in flight would be cut off"
        )

    if re.search(r"^Type=notify$", unit, re.M) and "NOTIFY_SOCKET" not in read_names:
        failures.append(
            "the unit is Type=notify and cmd/bot never reads NOTIFY_SOCKET; systemd would wait for "
            "a readiness message that cannot arrive and mark the start as failed"
        )

    if failures:
        print("FAIL check-binary-contract:")
        for failure in failures:
            print(f"  {failure}")
        return 1

    print(
        f"check-binary-contract: passed; {len(flags)} flags and {len(read_names)} environment names "
        f"in cmd/bot, the unit's ExecStart flag and both files' settings all match, and the stop "
        f"signal {expected} has a handler"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
