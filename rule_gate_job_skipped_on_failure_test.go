package jactionlint

import "testing"

func TestRuleGateJobSkippedOnFailure(t *testing.T) {
	const head = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    outputs:\n      v: ${{ steps.s.outputs.v }}\n    steps:\n      - id: s\n        run: echo \"v=1\" >> $GITHUB_OUTPUT\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	gate := func(job string) string {
		return head + "  final:\n    needs: [a, b]\n    runs-on: ubuntu-latest\n" + job
	}
	tests := []struct {
		what string
		src  string
		want []string
	}{
		{"result in a step", gate("    steps:\n      - run: test \"${{ contains(needs.*.result, 'failure') }}\" = false\n"), []string{"14"}},
		{"result in env", gate("    steps:\n      - run: echo\n        env:\n          R: ${{ needs.a.result }}\n"), []string{"14"}},
		{"whole needs", gate("    steps:\n      - run: echo\n        env:\n          R: ${{ toJSON(needs) }}\n"), []string{"14"}},
		{"index access", gate("    steps:\n      - run: echo\n        env:\n          R: ${{ needs['a'].result }}\n"), []string{"14"}},
		{"outcome", gate("    steps:\n      - run: echo\n        env:\n          R: ${{ needs.a.outcome }}\n"), []string{"14"}},
		{"not equal to success", gate("    steps:\n      - run: echo\n        if: needs.a.result != 'success'\n"), []string{"14"}},
		{"negated equal to success", gate("    steps:\n      - run: echo\n        if: ${{ !(needs.a.result == 'success') }}\n"), []string{"14"}},
		{"equal to failure in the job condition", head + "  final:\n    needs: a\n    if: needs.a.result == 'failure'\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", []string{"16"}},
		{"explicit success is the implicit one", gate("    if: success() && needs.a.result != 'skipped'\n    steps:\n      - run: echo\n"), []string{"17"}},
		{"step always does not run the job", gate("    steps:\n      - run: echo\n        if: always() && needs.a.result == 'failure'\n"), []string{"14"}},
		{"job output of a reusable workflow call", head + "  call:\n    needs: a\n    uses: ./.github/workflows/x.yml\n    with:\n      ok: ${{ needs.a.result }}\n", []string{"14"}},

		{"not cancelled", gate("    if: ${{ !cancelled() }}\n    steps:\n      - run: test \"${{ contains(needs.*.result, 'failure') }}\" = false\n"), nil},
		{"always", gate("    if: always()\n    steps:\n      - run: echo ${{ needs.a.result }}\n"), nil},
		{"always in an expression", gate("    if: ${{ always() && github.event_name == 'push' }}\n    steps:\n      - run: echo ${{ needs.a.result }}\n"), nil},
		{"failure", gate("    if: failure()\n    steps:\n      - run: echo ${{ needs.a.result }}\n"), nil},
		{"cancelled", gate("    if: cancelled()\n    steps:\n      - run: echo ${{ needs.a.result }}\n"), nil},
		{"negated failure", gate("    if: ${{ !failure() }}\n    steps:\n      - run: echo ${{ needs.a.result }}\n"), nil},
		{"only success is compared", gate("    steps:\n      - run: echo\n        if: needs.a.result == 'success'\n"), nil},
		{"success in the job condition", head + "  final:\n    needs: a\n    if: needs.a.result == 'success' && github.ref == 'refs/heads/main'\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", nil},
		{"outputs only", gate("    steps:\n      - run: echo ${{ needs.a.outputs.v }}\n"), nil},
		{"no needs", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ needs.a.result }}\n", nil},
		{"needed job with continue-on-error", "on: push\njobs:\n  a:\n    continue-on-error: true\n    runs-on: ubuntu-latest\n    steps:\n      - run: exit 1\n  final:\n    needs: a\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ needs.a.result }}\n", nil},
		{"unparsable expression", gate("    steps:\n      - run: echo ${{ needs.a.result == }}\n"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			checkLines(t, lintBatchH(t, "rules:\n  gate-job-skipped-on-failure: error\n", tc.src, "gate-job-skipped-on-failure"), tc.want...)
		})
	}
}
