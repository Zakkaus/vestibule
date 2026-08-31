# Vendored stylesheets

Copied verbatim from the shared design system's `app/` directory. **Do not edit
them here.** A local change is invisible at the next sync and turns into a
conflict nobody can explain; take the improvement upstream instead.

Copied 2026-08-31 23:38 AEST. Checksums at that moment:

```
9c4374cee10f5a0dfc4dfeb3345debc628c2ec7f8ddf8ef1e997d47d80775d7b  tokens.css
ef227eb8251d5b0c3bb1baaccdf103ed604dee368ae55acf03eaebf6e0b1dc5e  components.css
d9849f3fae4d8d0f08f9cbcd1100217c562b487f3c65f90650dcbc1b6b83994d  shell.css
```

Compare against the source to see whether either side has moved since:

```sh
for f in tokens components shell; do
  diff -q ~/design-system/app/$f.css web/src/styles/$f.css
done
```

The first copy of this drifted twice in one evening — once because an agent
improved a line while copying, once because the upstream file changed after the
copy was taken. Neither is visible without running the comparison.
