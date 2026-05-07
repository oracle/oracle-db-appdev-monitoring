## Build and test
- plain `go build` uses the godror driver; use `-tags goora` for the go-ora (no-CGO) driver

## Documentation
- docs live in the site/ directory
- do not modify the docs directory. these are generated files.

## Development
- after making changes to the code, update the changelog (site/docs/releases/changelog.md). If the change references a github issue, attribute the author of that issue following the convention in the changelog file.
- do not modify THIRD_PARTY_LICENSES.txt. this is a generated file
