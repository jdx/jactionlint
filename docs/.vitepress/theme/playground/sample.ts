export const defaultSource = `# Paste your workflow YAML to this code editor

on:
  push:
    branch: main
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo "\${{ github.event.head_commit.message }}"
      - run: echo "\${{ matrix.os }}"`;
