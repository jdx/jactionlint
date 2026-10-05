Playground for actionlint
=========================

This is a development directory for [actionlint playground](https://jactionlint.jdx.dev/).

The playground is built with HTML/CSS/TypeScript/Wasm. All dependencies are defined in `package.json` and managed by `npm`.
Tasks for development are defined in [`Makefile`](./Makefile).

## Tasks

```sh
# Install dependencies, build main.wasm, start serving the app at localhost:1234 using Python
make

# Install dependencies, build main.wasm
make build

# Install dependencies
make dep

# Run tests
make test

# Clean all built files and dependencies
make clean
```

## Lint

Sources are linted with [eslint](https://eslint.org/) with [typescript-eslint](https://github.com/typescript-eslint/typescript-eslint),
[prettier](https://prettier.io/) and [stylelint](https://stylelint.io/).

`lint` npm script applies all the liters:

```sh
npm run lint
```

## Deployment

Deployment to GitHub Pages is automated by the [Pages workflow](../.github/workflows/pages.yaml). `mise run pages` builds
the same site into `./playground-dist` locally. See [CONTRIBUTING.md](../CONTRIBUTING.md) for more details.
