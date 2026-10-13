package jactionlint

// FactsSchemaURL is where the JSON Schema of --export-facts is published.
const FactsSchemaURL = "https://raw.githubusercontent.com/jdx/jactionlint/main/schema/jactionlint-facts.json"

// FactsSchema returns the JSON Schema (draft-07) of the document of --export-facts as indented JSON. Like the schema
// of the findings it does not set additionalProperties: fields are added within a version, so a reader ignores the
// ones it does not know. TestFactsSchemaMatchesTypes keeps it in step with the types of facts.go.
func FactsSchema() []byte {
	obj := func(desc string, props jsonObject, required ...string) jsonObject {
		o := jsonObject{"type": "object", "description": desc, "properties": props}
		if len(required) > 0 {
			o["required"] = required
		}
		return o
	}
	scopes := func(desc string) jsonObject {
		return jsonObject{"type": "object", "description": desc, "additionalProperties": sEnum("The level of the scope.", "read", "write", "none", "unknown")}
	}
	loc := sDescribedRef("Where it is in the file.", "location")
	str := func(d string) jsonObject { return sStr(d) }

	defs := jsonObject{
		"location": obj("A position in a file: 1-based, the column counted in Unicode code points.", jsonObject{
			"line":   sInt("The line."),
			"column": sInt("The column."),
		}, "line", "column"),
		"stringFact": obj("A string of the file. When dynamic is true it has a ${{ }} expression, so its value is known only when the workflow runs, and value is the text as written.", jsonObject{
			"value":    str("The text."),
			"dynamic":  sBool("An expression decides the value. It is omitted when false."),
			"location": loc,
		}, "value"),
		"numberFact": obj("A number of the file, or an expression that decides it.", jsonObject{
			"value":      sNumber("The number. It is 0 when dynamic is true."),
			"dynamic":    sBool("An expression decides the value. It is omitted when false."),
			"expression": str("The expression, when dynamic."),
			"location":   loc,
		}, "value"),
		"inputFact": obj("A key of with: of a step or of a job that calls a reusable workflow.", jsonObject{
			"name":     str("The name of the input."),
			"value":    str("The value as written."),
			"dynamic":  sBool("An expression decides the value. It is omitted when false."),
			"location": loc,
		}, "name", "value"),
		"event": obj("An event of on:.", jsonObject{
			"name":      str("The name of the event."),
			"location":  loc,
			"types":     sStrings("The activity types. Empty or absent means the default types of the event."),
			"filters":   jsonObject{"type": "object", "description": "The filters by their key (branches, branches-ignore, tags, tags-ignore, paths, paths-ignore, workflows).", "additionalProperties": sStrings("The patterns.")},
			"schedules": sStrings("The cron expressions of the event schedule."),
			"inputs":    sStrings("The names of the inputs of workflow_dispatch and workflow_call."),
		}, "name"),
		"permissions": obj("The permissions of the token that a file declares at one level.", jsonObject{
			"declared": sBool("False when the file has no permissions: at this level."),
			"scopes":   scopes("The scopes by name. The scope * is read-all, write-all, or none for an empty mapping. unknown means that an expression decides it."),
			"location": loc,
		}, "declared"),
		"effectivePermissions": obj("The permissions that the token of a job has.", jsonObject{
			"source": sEnum("Where they come from. repository-setting means neither the job nor the workflow declares them, so the default token of the repository applies, which the file does not say: scopes is empty and that is unknown, not none.", "job", "workflow", "repository-setting"),
			"scopes": scopes("The scopes by name."),
		}, "source"),
		"concurrency": obj("A concurrency: of a workflow or a job.", jsonObject{
			"form":               sEnum("mapping (group and cancel-in-progress), group (only a group name, which cannot cancel) or expression (the whole key is an expression, so the group and the flag are unknown).", "mapping", "group", "expression"),
			"group":              sDescribedRef("The group.", "stringFact"),
			"cancel_in_progress": sEnum("unset means not written, so nothing is cancelled; unknown means an expression decides it.", "true", "false", "unset", "unknown"),
			"queue":              str("The queue strategy, when written."),
			"location":           loc,
		}, "form", "cancel_in_progress"),
		"uses": obj("A uses: value.", jsonObject{
			"uses":       str("The value as written."),
			"kind":       sEnum("What it names.", "action", "reusable-workflow", "docker", "local", "invalid"),
			"dynamic":    sBool("The value has an expression."),
			"repository": str("owner/repo of an action or a reusable workflow of another repository."),
			"ref":        str("What follows @ (the digest of a docker image)."),
			"ref_kind":   sEnum("What the ref is.", "full-sha", "short-sha", "semver-tag", "other-tag-or-branch", "digest", "none"),
			"path":       str("The canonical ./path of a local action or workflow."),
		}, "uses", "kind"),
		"step": obj("A step. A step of a parallel: group is listed after the group.", jsonObject{
			"index":          sInt("The position in the job, from 0."),
			"location":       loc,
			"id":             str("The id of the step."),
			"name":           sDescribedRef("The name.", "stringFact"),
			"kind":           sEnum("What the step does.", "run", "action", "parallel"),
			"if":             str("The condition as written, without ${{ }}. It is not evaluated."),
			"shell":          sDescribedRef("The shell of a run step, when written.", "stringFact"),
			"uses":           sDescribedRef("The action of an action step.", "uses"),
			"inputs":         sArray("The keys of with: of an action step.", sRef("inputFact")),
			"inputs_dynamic": sBool("The whole with: is an expression, or has an insert, so inputs is not all of them."),
		}, "index", "kind"),
		"jobUses": jsonObject{
			"description": "A job that calls a reusable workflow.",
			"allOf": []jsonObject{sRef("uses"), obj("", jsonObject{
				"secrets_inherit": sBool("secrets: inherit."),
				"secrets":         sStrings("The secrets passed by name."),
				"inputs":          sArray("The inputs passed with with:.", sRef("inputFact")),
			}, "secrets_inherit")},
		},
		"job": obj("A job.", jsonObject{
			"id":                    str("The ID of the job."),
			"location":              loc,
			"name":                  sDescribedRef("The name.", "stringFact"),
			"needs":                 sStrings("The jobs it needs."),
			"runs_on":               sStrings("The labels of runs-on."),
			"runs_on_dynamic":       sBool("An expression decides the labels, so runs_on is not all of them."),
			"permissions":           sDescribedRef("The permissions the job declares.", "permissions"),
			"effective_permissions": sDescribedRef("The permissions its token has.", "effectivePermissions"),
			"concurrency":           sDescribedRef("The job's concurrency.", "concurrency"),
			"environment":           sDescribedRef("The environment.", "stringFact"),
			"timeout_minutes":       sDescribedRef("The timeout.", "numberFact"),
			"cache_mode":            sDescribedRef("The cache-mode.", "stringFact"),
			"if":                    str("The condition as written, without ${{ }}. It is not evaluated."),
			"uses":                  sDescribedRef("Set when the job calls a reusable workflow.", "jobUses"),
			"steps":                 sArray("The steps.", sRef("step")),
		}, "id", "permissions", "effective_permissions"),
		"calledBy": obj("The local callers of a reusable workflow.", jsonObject{
			"callers": sStrings("The files of the repository that call it directly, sorted."),
			"events":  sStrings("The events of the workflows that reach it, directly or through other reusable workflows, without workflow_call. Sorted."),
			"known":   sBool("True when every chain of calls ends in a workflow of this repository with events of its own. False when nothing in the repository calls the workflow, or a caller is itself only called: another repository may call it on any event, and events is not all of them."),
		}, "callers", "events", "known"),
		"workflow": obj("A workflow file.", jsonObject{
			"path":          str("The path relative to the root of the repository, with / as the separator (relative to the working directory when the file is in no repository)."),
			"parsed":        sBool("False when the file is not valid YAML: it has no other fact then."),
			"syntax_errors": sInt("The number of syntax errors. The facts are what could be read."),
			"name":          sDescribedRef("The name.", "stringFact"),
			"events":        sArray("The events of on:.", sRef("event")),
			"callable":      sBool("The workflow has the event workflow_call."),
			"called_by":     sDescribedRef("Who calls a callable workflow in this repository. Absent when it is not callable or the file is in no repository.", "calledBy"),
			"permissions":   sDescribedRef("The permissions the workflow declares.", "permissions"),
			"concurrency":   sDescribedRef("The concurrency.", "concurrency"),
			"cache_mode":    sDescribedRef("The cache-mode.", "stringFact"),
			"jobs":          sArray("The jobs in the order of the source.", sRef("job")),
		}, "path", "parsed", "events", "callable", "permissions", "jobs"),
		"actionCaller": obj("A workflow that runs an action.", jsonObject{
			"workflow": str("The workflow file."),
			"via":      sStrings("The local actions and reusable workflows between the workflow and the action, outermost first. Empty when a step of the workflow runs the action."),
			"events":   sStrings("The events of the workflow, without workflow_call. Empty means they are not known."),
			"inputs":   sArray("What the steps of the workflow pass in with:. Given only when a step of the workflow runs the action itself.", sRef("inputFact")),
		}, "workflow", "events"),
		"action": obj("An action.yml.", jsonObject{
			"path":          str("The path relative to the root of the repository, with / as the separator."),
			"parsed":        sBool("False when the file is not valid YAML."),
			"syntax_errors": sInt("The number of syntax errors."),
			"name":          sDescribedRef("The name.", "stringFact"),
			"using":         str("The runtime: composite, docker, node20 and the like."),
			"inputs":        sStrings("The names of the inputs."),
			"called_by": obj("The local workflows that run the action. Absent when the file is in no repository.", jsonObject{
				"known":   sBool("False when no workflow of the repository runs the action: it may be published for other repositories, and its events are unknown."),
				"callers": sArray("The callers.", sRef("actionCaller")),
			}, "known", "callers"),
			"steps": sArray("The steps of a composite action.", sRef("step")),
		}, "path", "parsed"),
		"call": obj("A call of a local reusable workflow or action.", jsonObject{
			"kind":        sEnum("What is called.", "reusable-workflow", "action"),
			"from":        str("The file with the call."),
			"job":         str("The job with the call."),
			"step":        sInt("The index of the step, absent for a job that calls a workflow."),
			"location":    loc,
			"to":          str("The file or the directory of the callee, relative to the root of the repository."),
			"permissions": sDescribedRef("The permissions of the token of the calling job: what the callee can have at most.", "effectivePermissions"),
			"events":      sStrings("The events of the calling workflow, without workflow_call. Empty means that no event of its own is known."),
		}, "kind", "from", "to", "events"),
	}
	return marshalSchema(jsonObject{
		"$schema":     schemaDraft,
		"$id":         FactsSchemaURL,
		"title":       "jactionlint facts",
		"description": "The output of `jactionlint --export-facts`: what the workflows and actions of a repository declare and what the call graph knows, with positions and an explicit word wherever a value is unknown. Fields are never removed or renamed within a version; new fields can appear, so readers must ignore the fields they do not know. Generated by `jactionlint --print-facts-schema`.",
		"type":        "object",
		"properties": jsonObject{
			"version":      sInt("The version of the document. It changes only when a field changes meaning."),
			"tool":         str("jactionlint."),
			"tool_version": str("The version of jactionlint."),
			"workflows":    sArray("The workflow files.", sRef("workflow")),
			"actions":      sArray("The action.yml files.", sRef("action")),
			"calls":        sArray("The local calls of the exported files. The callee need not be in workflows or actions.", sRef("call")),
		},
		"required":    []string{"version", "tool", "tool_version", "workflows", "actions", "calls"},
		"definitions": defs,
	})
}
