package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// This file is the only place that knows the output shapes of the two tools. jactionlint v2 changes
// its JSON and adds SARIF, so retargeting the harness means editing parseJactionlint only.

// jactionlintArgs are the flags which make jactionlint print machine-readable output.
var jactionlintArgs = []string{"-format", "{{json .}}"}

// zizmorArgs are the flags after the zizmor command. The directory to audit is appended.
var zizmorArgs = []string{"--offline", "--persona", "pedantic", "--format", "sarif"}

// parseJactionlint reads the output of `jactionlint -format '{{json .}}'`. The v1 output has only a coarse
// "kind". When a stable "id" is present (v2) it wins.
func parseJactionlint(repo, repoDir string, out []byte) ([]Finding, error) {
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, nil
	}
	var raw []struct {
		Message  string `json:"message"`
		Filepath string `json:"filepath"`
		Line     int    `json:"line"`
		Kind     string `json:"kind"`
		ID       string `json:"id"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse jactionlint JSON: %w", err)
	}
	fs := make([]Finding, 0, len(raw))
	for _, r := range raw {
		rule := r.ID
		if rule == "" {
			rule = r.Kind
		}
		fs = append(fs, Finding{
			Repo:    repo,
			File:    normalizePath(repoDir, r.Filepath),
			Line:    r.Line,
			Rule:    rule,
			Message: r.Message,
		})
	}
	return fs, nil
}

// parseZizmor reads `zizmor --format sarif`. It returns the findings and the zizmor version if reported.
func parseZizmor(repo, repoDir string, out []byte) ([]Finding, string, error) {
	var doc struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Version string `json:"version"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string                `json:"ruleId"`
				Message   struct{ Text string } `json:"message"`
				Locations []struct {
					LogicalLocations []struct {
						Properties struct {
							Symbolic struct {
								Key struct {
									Local struct {
										VerbatimPath string `json:"verbatim_path"`
									} `json:"Local"`
								} `json:"key"`
							} `json:"symbolic"`
						} `json:"properties"`
					} `json:"logicalLocations"`
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, "", fmt.Errorf("parse zizmor SARIF: %w", err)
	}
	if len(doc.Runs) == 0 {
		return nil, "", fmt.Errorf("parse zizmor SARIF: no runs")
	}
	var fs []Finding
	version := ""
	for _, run := range doc.Runs {
		if version == "" {
			version = run.Tool.Driver.Version
		}
		for _, r := range run.Results {
			f := Finding{Repo: repo, Rule: strings.TrimPrefix(r.RuleID, "zizmor/"), Message: r.Message.Text}
			if len(r.Locations) > 0 {
				loc := r.Locations[0].PhysicalLocation
				// The artifact URI is relative to the enclosing git root, which differs from the audited
				// directory in worktrees and nested checkouts. The verbatim path is relative to the
				// audited directory.
				file := loc.ArtifactLocation.URI
				for _, l := range r.Locations[0].LogicalLocations {
					if p := l.Properties.Symbolic.Key.Local.VerbatimPath; p != "" {
						file = p
						break
					}
				}
				f.File = normalizePath(repoDir, file)
				f.Line = loc.Region.StartLine
			}
			fs = append(fs, f)
		}
	}
	return fs, version, nil
}
