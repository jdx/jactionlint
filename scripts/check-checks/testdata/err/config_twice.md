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
  shellcheck: off
```

Example config:

```yaml
rules:
  pyflakes: off
```

Output:

```
```

<!-- Skip playground link -->

