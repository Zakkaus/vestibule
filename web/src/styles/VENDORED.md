# Vendored stylesheets

Copied verbatim from the shared design system's `app/` directory. **Do not edit
them here.** A local change is invisible at the next sync and turns into a
conflict nobody can explain; take the improvement upstream instead.

Copied 2026-09-02 01:54 AEST. Checksums at that moment:

```
bb09e3ae814d4278df07972728bac10380c38f604363d47ecda65d9e9d088816  tokens.css
a29f24429583c0dfb9b99f105cc0a6dc6dddf1716386f8d72c517257b33149a8  components.css
1d03fb66515ed6e0e313345e8d4449f4c049d092726ad98b93b65b9ef95c99ed  shell.css
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
