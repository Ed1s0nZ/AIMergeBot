package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"strings"
)

func commentHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}
func commentMarker(namespace string, run Run) string {
	return "<!-- AIMergeBot:" + commentHash(fmt.Sprintf("%s:%d:%s:%s", namespace, run.ID, run.BaseSHA, run.HeadSHA)) + " -->"
}
func commentText(value string) string {
	// Render potential mentions and line-leading quick actions as literal text.
	// HTML entities alone are insufficient if a downstream Markdown filter decodes them.
	value = strings.ReplaceAll(value, "@", "＠")
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "/") {
			lines[i] = strings.Replace(line, "/", "／", 1)
		}
	}
	value = html.EscapeString(strings.Join(lines, "\n"))
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "#", "\\#", "!", "\\!").Replace(value)
}
func renderComment(namespace string, run Run, reviews []Review) (string, error) {
	byID := map[string]Review{}
	for _, review := range reviews {
		byID[review.FindingID] = review
	}
	var b strings.Builder
	b.WriteString(commentMarker(namespace, run))
	fmt.Fprintf(&b, "\n## AIMergeBot 审计 · Run #%d\n\nBASE `%s` · HEAD `%s`\n\n%s", run.ID, run.BaseSHA, run.HeadSHA, commentText(run.Result.Summary))
	for _, f := range run.Result.Findings {
		location := fmt.Sprintf("%s:%s:%d", f.Side, f.File, f.Line)
		if f.AnchorType == "git_metadata" {
			location = "Git metadata " + f.Side + ":" + f.File
		}
		review := byID[f.ID]
		label := map[string]string{"accepted": "已接受", "false_positive": "误报", "fixed": "已修复", "pending": "待复核"}[review.Status]
		if label == "" {
			label = "待复核"
		}
		fmt.Fprintf(&b, "\n\n### %s · %s\n\n位置：%s · 证据状态：%s · 复核：%s\n\n%s\n\n触发条件：%s\n\n建议：%s", commentText(f.Severity), commentText(f.Title), commentText(location), commentText(f.Confidence), label, commentText(f.Description), commentText(f.Trigger), commentText(f.Suggestion))
		if v := f.Verification; v != nil {
			verificationLabel := map[string]string{"supported": "独立复核支持", "rejected": "独立复核未支持", "inconclusive": "复核信息不足", "unavailable": "独立复核未完成", "disabled": "独立复核已关闭"}[v.Status]
			fmt.Fprintf(&b, "\n\n%s：%s（静态复核，非运行复现）", verificationLabel, commentText(v.Reason))
		}
		if review.Reason != "" {
			fmt.Fprintf(&b, "\n\n复核依据：%s", commentText(review.Reason))
		}
		if b.Len() > 64*1024 {
			return "", fmt.Errorf("comment body exceeds 64KiB budget")
		}
	}
	b.WriteString("\n\n审计结果需人工复核，不构成代码安全保证。")
	if b.Len() > 64*1024 {
		return "", fmt.Errorf("comment body exceeds 64KiB budget")
	}
	return b.String(), nil
}
