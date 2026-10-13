package jactionlint

import (
	"encoding/json"
	"testing"
)

// TestOutputSchemaCoversTheFindingContext: the output of --finding-context validates, and the schema describes the fields.
func TestOutputSchemaCoversTheFindingContext(t *testing.T) {
	v := loadJSONSchema(t, OutputSchema())
	finding := sub(t, v.root, "definitions", "finding", "properties")
	for _, k := range []string{"events", "permissions"} {
		if _, ok := finding[k]; !ok {
			t.Errorf("the output schema does not describe %q", k)
		}
	}
	_, out := findingContextRun(t, findingContextWorkflow, "--format", "json", "--finding-context")
	var doc any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if errs := v.validate(v.root, doc, "$"); len(errs) > 0 {
		t.Errorf("--finding-context does not validate: %v", errs)
	}
}
