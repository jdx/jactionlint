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

Example file: a.yml

```yaml
a: b
```

Example file: a.yml

```yaml
a: c
```

Output:

```
```

<!-- Skip playground link -->

