<a id="hello"></a>
## Hello

Example input:

```yaml
# select: no-such-rule-at-all
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ unknown }}
```

Output:

```
```

<!-- Skip playground link -->

