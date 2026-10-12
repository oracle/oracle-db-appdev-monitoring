# Release checklist

Use this checklist when preparing a new exporter release. Replace `<VERSION>`
with the release number (for example, `2.6.0`, without a `v` prefix), and use the
actual release date in the changelog.

Complete steps 1 and 2 before creating the documentation snapshot in step 3.

## 1. Update documentation and Compose image references

- [ ] Update exporter image tags in the current **Next** documentation under
  [`site/docs/`](site/docs/) to `<VERSION>`, including `docker pull`, `docker run`,
  Dockerfile `FROM` instructions, and other exporter image commands or examples.
- [ ] Update the exporter service image tag in
  [`docker-compose/compose.yaml`](docker-compose/compose.yaml) to `<VERSION>`.
- [ ] Keep unrelated image tags (such as Oracle Database, Prometheus, or Grafana)
  unchanged unless the release also requires their update.

From the repository root, locate exporter image references with:

```bash
rg -n 'container-registry.oracle.com/database/observability-exporter:' site/docs docker-compose/compose.yaml
```

Replace discovered tags with `<VERSION>`.

## 2. Date and version the changelog

- [ ] In [`site/docs/releases/changelog.md`](site/docs/releases/changelog.md),
  replace `### Next, TBD` with `### <VERSION>, <Month> <Day with ordinal>, <Year>`.
- [ ] Keep the release entries and contributor acknowledgments under that heading.
  Preserve the existing convention for attributing GitHub issue authors.

Follow the format used by previous releases, for example:

```markdown
### 2.5.0, August 31st, 2026
```

## 3. Create the versioned documentation

- [ ] From the `site/` directory, run:

```bash
cd site
npm run docusaurus docs:version <VERSION>
```

- [ ] Review the generated `site/versioned_docs/version-<VERSION>/` and
  `site/versioned_sidebars/version-<VERSION>-sidebars.json`, and confirm
  `site/versions.json` lists the new release first.
- [ ] Confirm the snapshot contains the updated image tags and dated changelog.
- [ ] After the snapshot is created, add a fresh `### Next, TBD` heading above the
  released section in the source `site/docs/releases/changelog.md` for future work.

## 4. Add the release to navigation and update indexing

In [`site/docusaurus.config.ts`](site/docusaurus.config.ts):

- [ ] Add `<VERSION>` immediately after `current` in the navbar
  `docsVersionDropdown.versions` list, keeping releases in newest-first order.
- [ ] Add `'<VERSION>': {noIndex: false}` to `presets → classic → docs → versions`.
- [ ] Change the previously latest release to `noIndex: true`.
- [ ] Keep `current` (displayed as **Next**) and every older release set to
  `noIndex: true`. Exactly one released version should have `noIndex: false`.

Rely on Docusaurus robots metadata and sitemap filtering for this policy;
`robots.txt` disallows do not remove pages from search indexes. Page descriptions
and topic keywords remain in `site/src/data/docs-seo.ts`; page titles come from
front matter or the first heading.

## 5. Update the default build version

- [ ] Update the repository-root [`Makefile`](Makefile) default:

```makefile
VERSION        ?= <VERSION>
```

- [ ] From the repository root, run `make version` and confirm it prints the new
  version without an override.

## Final checks

- [ ] From `site/`, run `npm run typecheck` and `npm run build`.
- [ ] Preview the built site with `npm run serve`: confirm the new release appears
  in the version dropdown and its examples and changelog are correct.
- [ ] Check the built HTML robots metadata and `site/build/sitemap.xml`: the new
  release is indexable, while Next and all older release routes have `noindex`
  and are excluded from the sitemap.
- [ ] Review `git status --short`, the release diff, and `git diff --check` from the
  repository root. Include the source edits and generated version snapshot in the
  release change, while preserving unrelated work.
