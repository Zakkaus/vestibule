# What these documents are

The flow reference for **the bot this repository is replacing**, moved here whole.
They describe v4's behaviour down to the function that implements each branch —
`internal/verify.(*Service).OnJoinRequest`, `internal/store/json.go`, and the rest
— and several of those packages no longer exist here.

They are kept because they are the most precise statement anyone has of what the
previous generation does, including its failure branches. The rewrite has to
match that behaviour or decide, in writing, not to. Read them as the
specification of what must not be lost, never as instructions for this program.

**Not maintained against v5.** Entry points named here go stale as the rewrite
proceeds, and that is expected: correcting them would turn a record of what was
into a second, worse copy of the architecture. When v5 replaces a behaviour
described here, the plan says so and the architecture explains why.

Nothing in this directory is the current deployment procedure. That lives in
`web/architecture.html` and, once phase nine builds it, in the install script.
