package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func queryKey(name string, args any) string {
	raw, _ := json.Marshal(args)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	delete(fields, "cursor")
	delete(fields, "page")
	raw, _ = json.Marshal(fields)
	return name + string(raw)
}
func (t *auditTools) unresolved() []ToolTrace {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []ToolTrace{}
	for _, v := range t.pending {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func textPage(text string, cursor int) (toolOutput, error) {
	if cursor < 0 || cursor > len(text) {
		return toolOutput{}, fmt.Errorf("invalid cursor")
	}
	end := cursor + 16000
	if end >= len(text) {
		return toolOutput{Text: text[cursor:]}, nil
	}
	// Cursor is a byte offset into this deterministic result. Preserve complete UTF-8 characters.
	for end > cursor && (text[end]&0xc0) == 0x80 {
		end--
	}
	return toolOutput{Text: text[cursor:end], More: true, NextCursor: end}, nil
}
func (t *auditTools) allFiles(ctx context.Context, base bool) ([]string, error) {
	if g, ok := t.repo.(*GitRepository); ok {
		return g.paths(ctx, t.snap, base)
	}
	if base {
		return nil, fmt.Errorf("base tree requires native Git")
	}
	out := []string{}
	for page := 1; page <= 20; page++ {
		files, more, e := t.repo.ListFiles(ctx, t.snap, page)
		if e != nil {
			return nil, e
		}
		out = append(out, files...)
		if !more {
			return out, nil
		}
	}
	return nil, fmt.Errorf("repository file listing exceeded API limit; enable native Git")
}
func validScope(p string) bool { return p == "" || p == "." || validPath(p) }
func scopeMatch(p, prefix, extension string) bool {
	return (prefix == "" || prefix == "." || p == prefix || strings.HasPrefix(p, prefix+"/")) && (extension == "" || path.Ext(p) == "."+strings.TrimPrefix(extension, "."))
}
func (t *auditTools) search(ctx context.Context, a searchArgs) (toolOutput, error) {
	return t.invoke("search_code", a, func() (toolOutput, error) {
		if len(a.Query) < 1 || len(a.Query) > 500 || !validScope(a.Path) {
			return toolOutput{}, fmt.Errorf("invalid query or path")
		}
		if a.Page > 1 {
			return toolOutput{}, fmt.Errorf("use next_cursor instead of page for complete search")
		}
		var text string
		if g, ok := t.repo.(*GitRepository); ok {
			ref, e := g.ref(t.snap, a.Base)
			if e != nil {
				return toolOutput{}, e
			}
			sizes, e := g.command(ctx, "ls-tree", "-r", "-l", "-z", ref)
			if e != nil {
				return toolOutput{}, e
			}
			if e = validateSearchSizes(sizes, a); e != nil {
				return toolOutput{}, e
			}
			flags := []string{"grep", "-I", "-n", "--no-color", "--no-textconv", "--threads=1", "-z"}
			if a.Regex {
				flags = append(flags, "-E")
			} else {
				flags = append(flags, "-F")
			}
			if a.IgnoreCase {
				flags = append(flags, "-i")
			}
			flags = append(flags, "-e", a.Query, ref, "--")
			if a.Path != "" && a.Path != "." {
				flags = append(flags, a.Path)
			}
			raw, e := g.command(ctx, flags...)
			if e != nil {
				return toolOutput{}, e
			}
			var b strings.Builder
			for raw != "" {
				name, rest, ok := strings.Cut(raw, "\x00")
				if !ok {
					return toolOutput{}, fmt.Errorf("invalid Git search output")
				}
				number, rest, ok := strings.Cut(rest, "\x00")
				if !ok {
					return toolOutput{}, fmt.Errorf("invalid Git search line")
				}
				content, next, _ := strings.Cut(rest, "\n")
				raw = next
				name = strings.TrimPrefix(name, ref+":")
				if !scopeMatch(name, "", a.Extension) {
					continue
				}
				fmt.Fprintf(&b, "%s:%s: %s\n", name, number, content)
			}
			text = b.String()
		} else {
			files, e := t.allFiles(ctx, a.Base)
			if e != nil {
				return toolOutput{}, e
			}
			pattern := regexp.QuoteMeta(a.Query)
			if a.Regex {
				pattern = a.Query
			}
			if a.IgnoreCase {
				pattern = "(?i)" + pattern
			}
			re, e := regexp.Compile(pattern)
			if e != nil {
				return toolOutput{}, e
			}
			var b strings.Builder
			for _, p := range files {
				if !scopeMatch(p, a.Path, a.Extension) {
					continue
				}
				if err := ctx.Err(); err != nil {
					return toolOutput{}, err
				}
				content, e := t.read(ctx, p, a.Base)
				if e != nil {
					return toolOutput{}, fmt.Errorf("search incomplete for %s: %w", p, e)
				}
				for n, line := range strings.Split(content, "\n") {
					if re.MatchString(line) {
						if b.Len()+len(line) > 8*1024*1024 {
							return toolOutput{}, fmt.Errorf("search output budget exceeded; narrow query")
						}
						fmt.Fprintf(&b, "%s:%d: %s\n", p, n+1, line)
					}
				}
			}
			text = b.String()
		}
		return textPage(text, a.Cursor)
	})
}

type directoryArgs struct {
	Path   string `json:"path"`
	Depth  int    `json:"depth"`
	Base   bool   `json:"base"`
	Cursor int    `json:"cursor"`
}

func (t *auditTools) directory(ctx context.Context, a directoryArgs) (toolOutput, error) {
	return t.invoke("list_directory", a, func() (toolOutput, error) {
		if !validScope(a.Path) {
			return toolOutput{}, fmt.Errorf("invalid path")
		}
		if a.Depth == 0 {
			a.Depth = 2
		}
		if a.Depth < 1 || a.Depth > 20 {
			return toolOutput{}, fmt.Errorf("depth must be 1–20")
		}
		files, e := t.allFiles(ctx, a.Base)
		if e != nil {
			return toolOutput{}, e
		}
		seen := map[string]bool{}
		prefix := strings.TrimSuffix(a.Path, "/")
		if prefix == "." {
			prefix = ""
		}
		for _, p := range files {
			if !scopeMatch(p, prefix, "") {
				continue
			}
			rel := p
			if prefix != "" {
				rel = strings.TrimPrefix(p, prefix+"/")
			}
			segments := strings.Split(rel, "/")
			for i := 1; i <= len(segments) && i <= a.Depth; i++ {
				entry := strings.Join(segments[:i], "/")
				if i < len(segments) {
					entry += "/"
				}
				seen[entry] = true
			}
		}
		entries := []string{}
		for p := range seen {
			entries = append(entries, p)
		}
		sort.Strings(entries)
		return textPage(strings.Join(entries, "\n"), a.Cursor)
	})
}

type batchArgs struct {
	Files []readArgs `json:"files"`
}

func (t *auditTools) batch(ctx context.Context, a batchArgs) (toolOutput, error) {
	return t.invoke("read_files", a, func() (toolOutput, error) {
		if len(a.Files) < 1 || len(a.Files) > 8 {
			return toolOutput{}, fmt.Errorf("batch must contain 1–8 files")
		}
		var b strings.Builder
		for _, f := range a.Files {
			o, e := t.file(ctx, f)
			if e != nil || o.Error != "" {
				return toolOutput{}, fmt.Errorf("batch read failed: %s", o.Error)
			}
			part := fmt.Sprintf("File %s (base=%t)\n%s", f.Path, f.Base, o.Text)
			if b.Len()+len(part) > 16000 {
				return toolOutput{}, fmt.Errorf("batch output budget exceeded; reduce batch or ranges")
			}
			b.WriteString(part)
			if o.More {
				return toolOutput{Text: b.String(), More: true}, nil
			}
		}
		return toolOutput{Text: b.String()}, nil
	})
}

// Git grep reads blobs directly; reject oversized candidate inputs before allocating them.
func validateSearchSizes(raw string, a searchArgs) error {
	for _, entry := range strings.Split(raw, "\x00") {
		header, p, ok := strings.Cut(entry, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(header)
		if len(fields) != 4 || fields[1] != "blob" || !scopeMatch(p, a.Path, a.Extension) {
			continue
		}
		size, e := strconv.ParseInt(fields[3], 10, 64)
		if e != nil {
			return fmt.Errorf("invalid Git blob size")
		}
		if size > 16*1024*1024 {
			return fmt.Errorf("search input exceeds 16 MiB per-blob budget: %s; narrow path", p)
		}
	}
	return nil
}
