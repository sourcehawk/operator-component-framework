// Package pluginsync copies the framework docs into the reference folders of the Claude Code plugin skills and
// rewrites their relative links, so that each link resolves in the plugin layout.
package pluginsync

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// SiteURL is the base URL of the published docs site. A link to a doc that the plugin does not bundle points there.
const SiteURL = "https://sourcehawk.github.io/operator-component-framework/"

// Copy maps a doc, or a glob of docs, relative to the docs folder, to its copy relative to the skills folder. Both use
// slash separators. The destination starts with <skill>/references. For a glob, the destination is a folder, and each
// match keeps its file name in it.
type Copy struct {
	Source string
	Dest   string
}

// Bundled lists the docs that the plugin bundles.
var Bundled = []Copy{
	{"component.md", "building-components/references/component.md"},
	{"observability.md", "building-components/references/observability.md"},
	{"primitives.md", "using-primitives/references/primitives.md"},
	{"primitives/*.md", "using-primitives/references/primitives"},
	{"custom-resource.md", "custom-resource-wrappers/references/custom-resource.md"},
	{"guidelines.md", "structuring-operators/references/guidelines.md"},
	{"compatibility.md", "structuring-operators/references/compatibility.md"},
	{"testing.md", "testing-operators/references/testing.md"},
}

// linkPatterns match the destination, in their first group, of an inline link or image with an optional title, and
// of a link reference definition.
var linkPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\]\(([^()\s]+)(?:\s+(?:"[^"]*"|'[^']*'))?\)`),
	regexp.MustCompile(`(?m)^ {0,3}\[[^\]]+\]:[ \t]*(\S+)`),
}

// Sync copies each doc of docs from docsDir to skillsDir. It first removes each <skill>/references folder under
// skillsDir, so a doc or a skill that is no longer bundled leaves no stale copy. In each copy, a link to another copied doc points
// to the copy of that doc, and a link to a doc that is not copied points to its page on the docs site at siteURL.
// Anchors are kept, and links inside code do not change. Sync fails for a link to a doc that does not exist in
// docsDir, and for a glob that matches no doc.
func Sync(docsDir, skillsDir, siteURL string, docs []Copy) error {
	copies, err := resolveCopies(docsDir, docs)
	if err != nil {
		return err
	}

	stale, err := filepath.Glob(filepath.Join(skillsDir, "*", "references"))
	if err != nil {
		return fmt.Errorf("finding reference folders: %w", err)
	}
	for _, dir := range stale {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("removing %s: %w", dir, err)
		}
	}

	for source, dest := range copies {
		content, err := os.ReadFile(filepath.Join(docsDir, source))
		if err != nil {
			return fmt.Errorf("reading %s: %w", source, err)
		}

		rewritten, err := rewriteLinks(string(content), source, docsDir, copies, siteURL)
		if err != nil {
			return fmt.Errorf("rewriting links in %s: %w", source, err)
		}

		target := filepath.Join(skillsDir, dest)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating the folder of %s: %w", dest, err)
		}
		if err := os.WriteFile(target, []byte(rewritten), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
	}

	return nil
}

// resolveCopies expands docs into one entry per doc, keyed by the doc path relative to docsDir.
func resolveCopies(docsDir string, docs []Copy) (map[string]string, error) {
	copies := make(map[string]string)
	for _, c := range docs {
		if !strings.ContainsAny(c.Source, "*?[") {
			copies[c.Source] = c.Dest
			continue
		}

		matches, err := filepath.Glob(filepath.Join(docsDir, filepath.FromSlash(c.Source)))
		if err != nil {
			return nil, fmt.Errorf("expanding %s: %w", c.Source, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("expanding %s: no docs match", c.Source)
		}
		for _, m := range matches {
			rel, err := filepath.Rel(docsDir, m)
			if err != nil {
				return nil, fmt.Errorf("expanding %s: %w", c.Source, err)
			}
			copies[filepath.ToSlash(rel)] = path.Join(c.Dest, filepath.Base(m))
		}
	}
	return copies, nil
}

// rewriteLinks rewrites each relative link to a Markdown doc in content, a doc at source in docsDir, for its copy in
// the plugin. It returns an error for a link to a doc that does not exist in docsDir.
func rewriteLinks(content, source, docsDir string, copies map[string]string, siteURL string) (string, error) {
	var errs []error
	rewritten := mapLinks(content, func(target string) string {
		file, anchor, _ := strings.Cut(target, "#")
		if anchor != "" {
			anchor = "#" + anchor
		}
		if !isRelative(file) || path.Ext(file) != ".md" {
			return target
		}

		doc := path.Join(path.Dir(source), file)
		if dest, ok := copies[doc]; ok {
			return relativePath(copies[source], dest) + anchor
		}

		if _, err := os.Stat(filepath.Join(docsDir, filepath.FromSlash(doc))); err != nil {
			errs = append(errs, fmt.Errorf("link to %s: %w", target, err))
			return target
		}
		return siteURL + pagePath(doc) + anchor
	})
	return rewritten, errors.Join(errs...)
}

// relativePath returns the slash path from the folder of the file from to the file to.
func relativePath(from, to string) string {
	fromParts := strings.Split(path.Dir(from), "/")
	toParts := strings.Split(to, "/")

	common := 0
	for common < len(fromParts) && common < len(toParts)-1 && fromParts[common] == toParts[common] {
		common++
	}

	ups := slices.Repeat([]string{".."}, len(fromParts)-common)
	return path.Join(append(ups, toParts[common:]...)...)
}

// pagePath returns the path of a doc on the docs site, which serves each page as a folder.
func pagePath(doc string) string {
	page := strings.TrimSuffix(doc, ".md")
	if page == "index" {
		return ""
	}
	return strings.TrimSuffix(page, "/index") + "/"
}

// CheckLinks walks the Markdown files under root and returns an error that names each relative destination, outside
// code, of an inline link or a link reference definition that resolves to no file.
func CheckLinks(root string) error {
	var errs []error
	err := filepath.WalkDir(root, func(file string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(file) != ".md" {
			return err
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		mapLinks(string(content), func(target string) string {
			linked, _, _ := strings.Cut(target, "#")
			if !isRelative(linked) {
				return target
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(file), filepath.FromSlash(linked))); err != nil {
				errs = append(errs, fmt.Errorf("%s: link to %s resolves to no file", file, target))
			}
			return target
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking %s: %w", root, err)
	}
	return errors.Join(errs...)
}

// isRelative reports whether a link target without its anchor is a path relative to the file that holds it.
func isRelative(target string) bool {
	return target != "" && !strings.HasPrefix(target, "/") && !strings.Contains(target, ":")
}

// mapLinks replaces the destination of each inline link and link reference definition in content with the result of
// fn. It does not change a link in a fenced code block or in an inline code span.
func mapLinks(content string, fn func(target string) string) string {
	var out, paragraph strings.Builder
	flush := func() {
		out.WriteString(mapOutsideCodeSpans(paragraph.String(), func(text string) string {
			for _, pattern := range linkPatterns {
				text = replaceFirstGroup(pattern, text, fn)
			}
			return text
		}))
		paragraph.Reset()
	}

	fence := ""
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			out.WriteString(line)
			if closesFence(trimmed, fence) {
				fence = ""
			}
		case fenceMarker(trimmed) != "":
			flush()
			fence = fenceMarker(trimmed)
			out.WriteString(line)
		case trimmed == "":
			flush()
			out.WriteString(line)
		default:
			paragraph.WriteString(line)
		}
	}
	flush()
	return out.String()
}

// replaceFirstGroup replaces the first group of each match of pattern in text with the result of fn.
func replaceFirstGroup(pattern *regexp.Regexp, text string, fn func(string) string) string {
	var out strings.Builder
	last := 0
	for _, m := range pattern.FindAllStringSubmatchIndex(text, -1) {
		out.WriteString(text[last:m[2]])
		out.WriteString(fn(text[m[2]:m[3]]))
		last = m[3]
	}
	out.WriteString(text[last:])
	return out.String()
}

// fenceMarker returns the run of backticks or tildes that opens a fenced code block on line, or "".
func fenceMarker(line string) string {
	for _, c := range []string{"`", "~"} {
		run := len(line) - len(strings.TrimLeft(line, c))
		if run >= 3 {
			return strings.Repeat(c, run)
		}
	}
	return ""
}

// closesFence reports whether line closes the fence that marker opened: a run of the same character, at least as long,
// with nothing after it.
func closesFence(line, marker string) bool {
	return strings.HasPrefix(line, marker) && strings.Trim(line, marker[:1]) == ""
}

// mapOutsideCodeSpans applies fn to each part of paragraph that is not in an inline code span.
func mapOutsideCodeSpans(paragraph string, fn func(text string) string) string {
	var out strings.Builder
	for {
		start := strings.Index(paragraph, "`")
		if start < 0 {
			out.WriteString(fn(paragraph))
			return out.String()
		}

		run := len(paragraph[start:]) - len(strings.TrimLeft(paragraph[start:], "`"))
		delimiter := paragraph[start : start+run]
		end := closingDelimiter(paragraph[start+run:], delimiter)
		if end < 0 {
			out.WriteString(fn(paragraph))
			return out.String()
		}

		spanEnd := start + run + end + run
		out.WriteString(fn(paragraph[:start]))
		out.WriteString(paragraph[start:spanEnd])
		paragraph = paragraph[spanEnd:]
	}
}

// closingDelimiter returns the index in s of a run of backticks equal to delimiter, or -1.
func closingDelimiter(s, delimiter string) int {
	for i := 0; i < len(s); {
		j := strings.Index(s[i:], delimiter)
		if j < 0 {
			return -1
		}
		at := i + j
		if !strings.HasPrefix(s[at+len(delimiter):], "`") && (at == 0 || s[at-1] != '`') {
			return at
		}
		i = at + len(delimiter)
		for i < len(s) && s[i] == '`' {
			i++
		}
	}
	return -1
}
