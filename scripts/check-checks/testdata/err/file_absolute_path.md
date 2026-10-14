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

Example file: /etc/outside.yml

```yaml
a: b
```

Output:

```
```

<!-- Skip playground link -->

