## Build and test
- plain `go build` uses the godror driver; use `-tags goora` for the go-ora (no-CGO) driver

## Documentation
- docs live in the site/ directory
- do not modify the docs directory. these are generated files.

## Site SEO and indexing
- Keep page descriptions and topic keywords in `site/src/data/docs-seo.ts`; page titles come from doc front matter or the first heading.
- Mark the current Docusaurus docs version (`current`, shown as “Next”) with `noIndex: true`.
- Keep only the latest released docs version indexable. Mark every older version with `noIndex: true` in `site/docusaurus.config.ts`.
- When promoting a release, set its version to `noIndex: false` and mark the previously latest release `noIndex: true`.
- Rely on Docusaurus' robots metadata and sitemap filtering for these rules; `robots.txt` disallows do not remove pages from search indexes.

## Development
- after making changes to the code, update the changelog (site/docs/releases/changelog.md). If the change references a github issue, attribute the author of that issue following the convention in the changelog file.
- do not modify THIRD_PARTY_LICENSES.txt. this is a generated file
