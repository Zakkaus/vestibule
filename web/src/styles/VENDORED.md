# Vendored stylesheets

Copied verbatim from the shared design system's `app/` directory. **Do not edit
them here.** A local change is invisible at the next sync and turns into a
conflict nobody can explain; take the improvement upstream instead.

Copied 2026-09-01 20:54 AEST. Checksums at that moment:

```
5af84e445428dba07fb7e81bc8ed6722da30d376e6aacac669ec0a96ce2470f3  tokens.css
a29f24429583c0dfb9b99f105cc0a6dc6dddf1716386f8d72c517257b33149a8  components.css
99767f2dd32ea0ac57bb4b7f994736cf5727ae7ee1937c7acdb02717f5a7fd12  shell.css
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
