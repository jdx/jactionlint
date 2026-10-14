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


Output:

```
```

<!-- Skip playground link -->

Example config:

```yaml
rules:
  shellcheck: off
```
