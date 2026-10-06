package platform

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type sarifObject = map[string]any

// BuildSARIF exports saved static evidence, never newly inferred code flows.
func BuildSARIF(run Run, reviews []Review) sarifObject {
	headProject := run.SourceProjectID
	if headProject == 0 {
		headProject = run.ProjectID
	}
	bases := sarifObject{
		"BASE": sarifObject{"uri": snapshotURI(run.ProjectID, run.BaseSHA)},
		"HEAD": sarifObject{"uri": snapshotURI(headProject, run.HeadSHA)},
	}
	shas := map[string]string{"BASE": run.BaseSHA, "HEAD": run.HeadSHA}
	for _, source := range contextPolicyItems(run.Snapshot) {
		bases[fmt.Sprintf("CONTEXT_%d", source.ProjectID)] = sarifObject{"uri": snapshotURI(source.ProjectID, source.SHA)}
		shas[fmt.Sprintf("CONTEXT_%d", source.ProjectID)] = source.SHA
	}
	types := map[string]bool{}
	for _, f := range run.Result.Findings {
		types[sarifCategory(f.Type)] = true
	}
	categories := []string{}
	for category := range types {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	rules := []sarifObject{}
	indices := map[string]int{}
	for i, category := range categories {
		indices[category] = i
		rules = append(rules, sarifObject{"id": sarifRuleID(category), "shortDescription": sarifObject{"text": category}})
	}
	results := []sarifObject{}
	for _, f := range run.Result.Findings {
		category := sarifCategory(f.Type)
		verification := "unverified"
		if f.Verification != nil {
			verification = f.Verification.Status
		}
		message := strings.TrimSpace(f.Title + "\n" + f.Description)
		if message == "" {
			message = "Saved PR audit finding"
		}
		properties := sarifObject{"findingId": f.ID, "confidence": f.Confidence, "verificationStatus": verification, "verification": f.Verification, "runtimeReproduced": false, "trigger": f.Trigger, "suggestion": f.Suggestion, "observationIds": f.ObservationIDs, "prContext": f.PRContext}
		result := sarifObject{"ruleId": sarifRuleID(category), "ruleIndex": indices[category], "level": sarifLevel(f.Severity), "message": sarifObject{"text": message}, "properties": properties}
		side := "HEAD"
		if f.Side == "base" {
			side = "BASE"
		}
		line := f.Line
		if f.AnchorType == "git_metadata" {
			line = 0
		}
		if validPath(f.File) {
			result["locations"] = []sarifObject{sarifLocation(f.File, side, line, f.Evidence)}
		}
		if f.Fingerprint != "" {
			result["partialFingerprints"] = sarifObject{"aimangebot/v1": f.Fingerprint}
		}
		related := []sarifObject{}
		omitted := 0
		if f.SequenceDiagram != nil {
			relationships := []sarifObject{}
			properties["sequenceLimitations"] = f.SequenceDiagram.Limitations
			for _, step := range f.SequenceDiagram.Steps {
				relationships = append(relationships, sarifObject{"label": step.Label, "certainty": step.Certainty, "kind": step.Kind})
				for _, ref := range step.Evidence {
					base := "HEAD"
					if ref.Side == "base" {
						base = "BASE"
					}
					if ref.RepositoryID != 0 {
						base = fmt.Sprintf("CONTEXT_%d", ref.RepositoryID)
					}
					if _, ok := bases[base]; !ok || !validPath(ref.File) || (ref.SHA != "" && ref.SHA != shas[base]) || (ref.Side != "base" && ref.Side != "head") || (ref.RepositoryID != 0 && ref.Side != "head") {
						omitted++
						continue
					}
					line := ref.Line
					if ref.AnchorType == "git_metadata" {
						line = 0
					}
					location := sarifLocation(ref.File, base, line, ref.Snippet)
					location["id"] = len(related) + 1
					location["message"] = sarifObject{"text": "Saved static source citation; not proof of call semantics"}
					related = append(related, location)
				}
			}
			properties["staticRelationships"] = relationships
		}
		if len(related) > 0 {
			result["relatedLocations"] = related
		}
		if omitted > 0 {
			properties["unmappedSourceReferences"] = omitted
		}
		for _, review := range reviews {
			if review.FindingID != f.ID {
				continue
			}
			properties["humanReview"] = review
			if review.Status == "false_positive" {
				reason := strings.TrimSpace(review.Reason)
				if reason == "" {
					reason = "Marked false positive by a human reviewer"
				}
				result["suppressions"] = []sarifObject{{"kind": "external", "status": "accepted", "justification": reason}}
			}
			break
		}
		results = append(results, result)
	}
	notifications := []sarifObject{}
	for _, note := range run.Result.CoverageNotes {
		if strings.TrimSpace(note) != "" {
			notifications = append(notifications, sarifObject{"level": "warning", "message": sarifObject{"text": note}})
		}
	}
	return sarifObject{"$schema": "https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json", "version": "2.1.0", "runs": []sarifObject{{
		"tool":               sarifObject{"driver": sarifObject{"name": "AIMergeBot", "version": run.PolicyVersion, "rules": rules}},
		"originalUriBaseIds": bases, "results": results,
		"invocations": []sarifObject{{"executionSuccessful": run.Status == "succeeded" && run.Error == "", "toolExecutionNotifications": notifications}},
		"properties":  sarifObject{"runId": run.ID, "status": run.Status, "error": run.Error, "baseSHA": run.BaseSHA, "headSHA": run.HeadSHA, "policyVersion": run.PolicyVersion, "coverageNotes": run.Result.CoverageNotes, "summary": run.Result.Summary, "runtimeReproduced": false, "investigationClaimReviews": sarifClaimReviews(run.Result.Investigations)},
	}}}
}

func sarifClaimReviews(items []Investigation) []sarifObject {
	rows := []sarifObject{}
	for _, item := range items {
		if item.ClaimVerification != nil {
			rows = append(rows, sarifObject{"investigationId": item.ID, "verification": cloneClaimVerification(item.ClaimVerification)})
		}
	}
	return rows
}

func snapshotURI(repository int, sha string) string {
	return fmt.Sprintf("aimangebot://repository/%d/commit/%s/", repository, url.PathEscape(sha))
}
func sarifCategory(category string) string {
	if strings.TrimSpace(category) == "" {
		return "security-risk"
	}
	return category
}
func sarifRuleID(category string) string {
	return fmt.Sprintf("AIMB-%x", sha256.Sum256([]byte(category)))
}
func sarifLevel(severity string) string {
	switch severity {
	case "critical", "high":
		return "error"
	case "low":
		return "note"
	default:
		return "warning"
	}
}
func sarifLocation(file, base string, line int, snippet string) sarifObject {
	uri := (&url.URL{Path: file}).String()
	physical := sarifObject{"artifactLocation": sarifObject{"uri": uri, "uriBaseId": base}}
	if line > 0 {
		region := sarifObject{"startLine": line}
		if snippet != "" {
			region["snippet"] = sarifObject{"text": snippet}
		}
		physical["region"] = region
	}
	return sarifObject{"physicalLocation": physical}
}
