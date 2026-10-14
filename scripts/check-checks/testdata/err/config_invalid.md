<a id="hello"></a>
## Hello

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ unknown }}
```

Example config:

```yaml
rules:
  no-such-rule-at-all: warn
  : [
```

Output:

```
```

<!-- Skip playground link -->

