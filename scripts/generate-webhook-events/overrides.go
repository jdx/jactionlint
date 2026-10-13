package main

import "slices"

// typeOverrides lists the activity types that the workflow schema of GitHub accepts but the documentation page
// does not list (yet). The generator reads the documentation, which lags behind the schema, so a type that is
// missing there would be reported as invalid although GitHub accepts it. The overrides are applied to the parsed
// documentation every time the file is generated, so they survive `go generate`.
//
// Add an entry only with a source that shows that GitHub accepts the type. An override of a type that the
// documentation lists has no effect, so an entry can stay when the documentation catches up.
//
// The sources are the schema of the language service, https://github.com/actions/languageservices
// (workflow-parser/src/workflow-v1.0.json), and the one of the runner, https://github.com/actions/runner
// (src/Sdk/WorkflowParser/workflow-v1.0.json).
var typeOverrides = map[string][]string{
	// https://github.com/actions/languageservices/pull/384 ("The `stacked` activity type is now available on
	// `pull_request` and `pull_request_target` events for Actions.")
	"pull_request":        {"stacked"},
	"pull_request_target": {"stacked"},
	// `image-version-activity-type` of the schema allows created, ready and deleted. The documentation says
	// "Not applicable" for the activity types of the event.
	"image_version": {"created", "ready", "deleted"},
}

// applyOverrides appends the types of overrides which the parsed documentation does not have yet. An event
// that is not in the documentation is left out: it is not known whether the event still exists.
func applyOverrides(parsed map[string][]string, overrides map[string][]string) {
	for event, extra := range overrides {
		types, ok := parsed[event]
		if !ok {
			dbg.Printf("Skipped the overrides of %q because the event is not in the documentation\n", event)
			continue
		}
		if types == nil {
			// "Custom" types: any type is valid, so there is nothing to add
			continue
		}
		for _, t := range extra {
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
		parsed[event] = types
	}
}
