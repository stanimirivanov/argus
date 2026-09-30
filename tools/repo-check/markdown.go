package main

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	atxHeadingPattern          = regexp.MustCompile(`^(#{1,6})[\t ]+(.+?)[\t ]*#*[\t ]*$`)
	wordPattern                = regexp.MustCompile(`[\p{L}\p{N}]+(?:['_-][\p{L}\p{N}]+)*`)
	htmlTagPattern             = regexp.MustCompile(`<[^>]+>`)
	inlineLinkPattern          = regexp.MustCompile(`!?\[([^]]*)\]\([\t ]*(<[^>]+>|[^\t )]+)(?:[\t ]+[^)]*)?\)`)
	referenceDefinitionPattern = regexp.MustCompile(
		`^[ ]{0,3}\[[^]]+\]:[\t ]*(<[^>]+>|[^\t ]+)(?:[\t ]+(?:"[^"]*"|'[^']*'|\([^)]*\)))?[\t ]*$`,
	)
)

type markdownDocument struct {
	path         string
	lines        []string
	headings     []markdownHeading
	anchors      map[string]int
	links        []markdownLink
	wordCount    int
	secondLevels []markdownHeading
}

type markdownHeading struct {
	level int
	text  string
	line  int
}

type markdownLink struct {
	destination string
	line        int
}

func parseMarkdown(path, content string) *markdownDocument {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	document := &markdownDocument{
		path:    filepath.ToSlash(path),
		lines:   strings.Split(content, "\n"),
		anchors: make(map[string]int),
	}
	anchorCounts := make(map[string]int)
	inFence := false
	var fenceMarker byte
	var fenceWidth int

	for index, line := range document.lines {
		trimmed := strings.TrimLeft(line, " \t")
		if marker, width, ok := fence(trimmed); ok {
			if !inFence {
				inFence = true
				fenceMarker = marker
				fenceWidth = width
			} else if marker == fenceMarker && width >= fenceWidth {
				inFence = false
			}

			continue
		}
		if inFence {
			continue
		}

		if reference, found := referenceDefinitionOnLine(line, index+1); found {
			document.links = append(document.links, reference)
		}

		prose := stripInlineCode(line)
		document.wordCount += len(wordPattern.FindAllString(prose, -1))
		document.links = append(document.links, linksOnLine(prose, index+1)...)

		match := atxHeadingPattern.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}
		heading := markdownHeading{
			level: len(match[1]),
			text:  strings.TrimSpace(match[2]),
			line:  index + 1,
		}
		document.headings = append(document.headings, heading)
		if heading.level == 2 {
			document.secondLevels = append(document.secondLevels, heading)
		}

		baseAnchor := githubAnchor(heading.text)
		anchor := baseAnchor
		if duplicate := anchorCounts[baseAnchor]; duplicate > 0 {
			anchor += "-" + decimal(duplicate)
		}
		anchorCounts[baseAnchor]++
		document.anchors[anchor] = heading.line
	}

	return document
}

func referenceDefinitionOnLine(line string, lineNumber int) (markdownLink, bool) {
	match := referenceDefinitionPattern.FindStringSubmatch(line)
	if match == nil {
		return markdownLink{}, false
	}

	destination := strings.TrimPrefix(match[1], "<")
	destination = strings.TrimSuffix(destination, ">")

	return markdownLink{destination: destination, line: lineNumber}, true
}

func fence(line string) (byte, int, bool) {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return 0, 0, false
	}
	marker := line[0]
	width := 0
	for width < len(line) && line[width] == marker {
		width++
	}
	if width < 3 {
		return 0, 0, false
	}

	return marker, width, true
}

func stripInlineCode(line string) string {
	var result strings.Builder
	inCode := false
	for index := 0; index < len(line); index++ {
		if line[index] == '`' {
			inCode = !inCode
			result.WriteByte(' ')
			continue
		}
		if !inCode {
			result.WriteByte(line[index])
		}
	}

	return result.String()
}

func linksOnLine(line string, lineNumber int) []markdownLink {
	matches := inlineLinkPattern.FindAllStringSubmatch(line, -1)
	links := make([]markdownLink, 0, len(matches))
	for _, match := range matches {
		destination := strings.TrimSpace(match[2])
		destination = strings.TrimPrefix(destination, "<")
		destination = strings.TrimSuffix(destination, ">")
		links = append(links, markdownLink{destination: destination, line: lineNumber})
	}

	return links
}

func githubAnchor(heading string) string {
	heading = inlineLinkPattern.ReplaceAllString(heading, "$1")
	heading = htmlTagPattern.ReplaceAllString(heading, "")
	heading = strings.ToLower(heading)

	var slug strings.Builder
	previousSpace := false
	for _, character := range heading {
		switch {
		case unicode.IsLetter(character), unicode.IsDigit(character), character == '_', character == '-':
			if previousSpace && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			previousSpace = false
			slug.WriteRune(character)
		case unicode.IsSpace(character):
			previousSpace = true
		}
	}

	return slug.String()
}

func decimal(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}

	return string(digits[position:])
}

func (r *repository) checkMarkdownLinks() []violation {
	paths := make([]string, 0, len(r.documents))
	for path := range r.documents {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var violations []violation
	for _, path := range paths {
		document := r.documents[path]
		for _, link := range document.links {
			if violation, found := r.checkMarkdownLink(document, link); found {
				violations = append(violations, violation)
			}
		}
	}

	return violations
}

func (r *repository) checkMarkdownLink(source *markdownDocument, link markdownLink) (violation, bool) {
	parsed, err := url.Parse(link.destination)
	if err != nil {
		return linkViolation(source, link, "is not a valid link target", "use a valid relative path and optional heading anchor"), true
	}
	if parsed.Scheme != "" || parsed.Host != "" || strings.HasPrefix(link.destination, "//") {
		return violation{}, false
	}

	decodedPath, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return linkViolation(source, link, "contains invalid path escaping", "percent-encode the relative path correctly"), true
	}
	decodedAnchor, err := url.PathUnescape(parsed.Fragment)
	if err != nil {
		return linkViolation(source, link, "contains invalid anchor escaping", "percent-encode the heading anchor correctly"), true
	}

	targetRelative := filepath.FromSlash(decodedPath)
	if filepath.IsAbs(targetRelative) || strings.HasPrefix(decodedPath, "/") {
		targetRelative = strings.TrimLeft(targetRelative, string(filepath.Separator))
	} else {
		targetRelative = filepath.Join(filepath.Dir(filepath.FromSlash(source.path)), targetRelative)
	}
	targetRelative = filepath.Clean(targetRelative)
	if targetRelative == "." && decodedPath == "" {
		targetRelative = filepath.FromSlash(source.path)
	}

	targetAbsolute := filepath.Join(r.root, targetRelative)
	inside, err := filepath.Rel(r.root, targetAbsolute)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return linkViolation(source, link, "escapes the repository root", "link to a checked-in repository path"), true
	}

	if !pathExistsWithExactCase(r.root, targetRelative) {
		return linkViolation(
			source,
			link,
			"does not resolve to a checked-in path with matching letter case",
			"correct the relative path or its capitalization",
		), true
	}
	if decodedAnchor == "" {
		return violation{}, false
	}

	targetPath := filepath.ToSlash(targetRelative)
	target, markdownTarget := r.documents[targetPath]
	if !markdownTarget {
		return violation{}, false
	}
	if _, found := target.anchors[decodedAnchor]; !found {
		return linkViolation(
			source,
			link,
			"references a heading anchor that does not exist",
			"use one of the target document's generated heading anchors",
		), true
	}

	return violation{}, false
}

func linkViolation(source *markdownDocument, link markdownLink, message, fix string) violation {
	return violation{
		path:    source.path,
		line:    link.line,
		rule:    "docs.internal-link",
		message: "link " + quote(link.destination) + " " + message,
		fix:     fix,
		policy:  documentationPolicy,
	}
}

func quote(value string) string {
	return `"` + value + `"`
}

func pathExistsWithExactCase(root, relative string) bool {
	clean := filepath.Clean(relative)
	if clean == "." {
		return true
	}

	current := root
	for _, segment := range strings.Split(clean, string(filepath.Separator)) {
		entries, err := os.ReadDir(current)
		if err != nil {
			return false
		}
		found := false
		for _, entry := range entries {
			if entry.Name() == segment {
				found = true
				break
			}
		}
		if !found {
			return false
		}
		current = filepath.Join(current, segment)
	}

	return true
}

func (r *repository) checkDocumentSummaries() []violation {
	var violations []violation
	for _, document := range r.documents {
		if !requiresSummary(document) {
			continue
		}
		if len(document.secondLevels) > 0 && document.secondLevels[0].text == "TL;DR" {
			continue
		}

		line := 1
		if len(document.secondLevels) > 0 {
			line = document.secondLevels[0].line
		}
		violations = append(violations, violation{
			path:    document.path,
			line:    line,
			rule:    "docs.tldr",
			message: "long or selectively consulted documentation does not start with a `## TL;DR` section after its title and metadata",
			fix:     "add `## TL;DR` as the first second-level section and summarize purpose, audience, outcome, and next action",
			policy:  documentationPolicy,
		})
	}

	return violations
}

func requiresSummary(document *markdownDocument) bool {
	if strings.HasPrefix(document.path, ".github/") {
		return false
	}
	if document.wordCount >= 800 || len(document.secondLevels) > 5 {
		return true
	}

	lowerPath := strings.ToLower(document.path)

	return strings.HasPrefix(lowerPath, "docs/architecture/") ||
		lowerPath == "security.md" ||
		strings.Contains(lowerPath, "migration") ||
		strings.Contains(lowerPath, "workflow") ||
		strings.Contains(lowerPath, "operations") ||
		strings.Contains(lowerPath, "runbook")
}
