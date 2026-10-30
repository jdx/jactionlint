# Expression corpus

`corpus.json` lists `(expression, context) -> result` cases. The `source` field says where each value comes from:

- `docs`: stated by the GitHub documentation of expressions (the coercion table, the function examples).
- `runner`: follows the source of actions/runner (`ExpressionUtility`, `EvaluationResult`), read by hand.

None of the values was recorded from a live GitHub run yet. Cases recorded from real runs belong here with
`"source": "run:<url>"`.
