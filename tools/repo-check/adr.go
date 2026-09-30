package main

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	adrFilePattern      = regexp.MustCompile(`^(\d{4})-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)
	adrHeadingPattern   = regexp.MustCompile(`^# ADR-(\d{4}): (\S.*)$`)
	adrListMetaPattern  = regexp.MustCompile(`^- ([A-Za-z][A-Za-z ]+):[\t ]*(.*?)[\t ]*$`)
	adrBoldMetaPattern  = regexp.MustCompile(`^\*\*([A-Za-z][A-Za-z ]+):\*\*[\t ]*(.*?)[\t ]*$`)
	adrIndexRowPattern  = regexp.MustCompile(`^\| \[ADR-(\d{4})\]\(([^)]+)\) \| ([^|]+) \| ([^|]+) \| ([^|]+) \|$`)
	milestonePattern    = regexp.MustCompile(`^M\d{2} - \S.*$`)
	adrReferencePattern = regexp.MustCompile(`ADR-(\d{4})`)
)

var allowedADRStatuses = map[string]struct{}{
	"Accepted":   {},
	"Deprecated": {},
	"Proposed":   {},
	"Rejected":   {},
	"Superseded": {},
}

// ADRs 0014-0017 predate the complete list-style metadata template. They are
// immutable accepted records, so the checker validates their status and date
// without pretending that absent historical metadata was recorded at the time.
var legacyADRMetadata = map[string]struct{}{
	"0014": {},
	"0015": {},
	"0016": {},
	"0017": {},
}

type adrRecord struct {
	number   string
	file     string
	title    string
	heading  int
	metadata map[string]metadataValue
}

type metadataValue struct {
	value string
	line  int
}

type adrIndexEntry struct {
	number string
	file   string
	title  string
	status string
	date   string
	line   int
}

func (r *repository) checkDecisions() []violation {
	records, violations := r.readADRRecords()
	index, indexViolations := r.readADRIndex()
	violations = append(violations, indexViolations...)
	violations = append(violations, validateADRSequence(records)...)
	violations = append(violations, validateADRIndex(records, index)...)
	violations = append(violations, validateADRSupersession(records)...)

	return violations
}

func (r *repository) readADRRecords() (map[string]adrRecord, []violation) {
	records := make(map[string]adrRecord)
	var violations []violation
	for path, document := range r.documents {
		if filepath.ToSlash(filepath.Dir(path)) != "docs/decisions" || filepath.Base(path) == "README.md" {
			continue
		}

		fileMatch := adrFilePattern.FindStringSubmatch(filepath.Base(path))
		if fileMatch == nil {
			violations = append(violations, violation{
				path:    path,
				line:    1,
				rule:    "adr.filename",
				message: "ADR filename does not use a four-digit number and lowercase kebab-case description",
				fix:     "rename it to `NNNN-short-decision-name.md` and update the ADR index and inbound links",
				policy:  decisionPolicy,
			})

			continue
		}

		record, recordViolations := parseADR(document, fileMatch[1])
		violations = append(violations, recordViolations...)
		if existing, duplicate := records[fileMatch[1]]; duplicate {
			violations = append(violations, violation{
				path:    path,
				line:    1,
				rule:    "adr.number",
				message: "ADR number duplicates " + quote(existing.file),
				fix:     "refresh the decision index and renumber the later ADR to the lowest unused number",
				policy:  decisionPolicy,
			})

			continue
		}
		records[fileMatch[1]] = record
	}

	return records, violations
}

func parseADR(document *markdownDocument, fileNumber string) (adrRecord, []violation) {
	record := adrRecord{
		number:   fileNumber,
		file:     document.path,
		heading:  1,
		metadata: make(map[string]metadataValue),
	}
	var violations []violation
	if len(document.lines) == 0 {
		return record, []violation{adrViolation(document.path, 1, "adr.heading", "ADR is empty", "add the canonical ADR heading and metadata")}
	}

	headingMatch := adrHeadingPattern.FindStringSubmatch(document.lines[0])
	if headingMatch == nil {
		violations = append(violations, adrViolation(
			document.path,
			1,
			"adr.heading",
			"ADR heading does not match `# ADR-NNNN: Decision title`",
			"make the first line use the canonical heading with the filename's number",
		))
	} else {
		record.number = headingMatch[1]
		record.title = strings.TrimSpace(headingMatch[2])
		if headingMatch[1] != fileNumber {
			violations = append(violations, adrViolation(
				document.path,
				1,
				"adr.number",
				"heading number ADR-"+headingMatch[1]+" does not match filename number "+fileNumber,
				"use ADR-"+fileNumber+" in the heading or correctly renumber the file and index",
			))
		}
	}

	firstSectionLine := len(document.lines) + 1
	if len(document.secondLevels) > 0 {
		firstSectionLine = document.secondLevels[0].line
	}
	for index := 1; index < firstSectionLine-1; index++ {
		name, value, found := parseADRMetadata(document.lines[index])
		if !found {
			continue
		}
		if previous, duplicate := record.metadata[name]; duplicate {
			violations = append(violations, adrViolation(
				document.path,
				index+1,
				"adr.metadata",
				"metadata field "+quote(name)+" duplicates line "+strconv.Itoa(previous.line),
				"keep exactly one "+name+" field before the first section",
			))

			continue
		}
		record.metadata[name] = metadataValue{value: value, line: index + 1}
	}

	violations = append(violations, validateADRMetadata(record)...)

	return record, violations
}

func parseADRMetadata(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	match := adrListMetaPattern.FindStringSubmatch(line)
	if match == nil {
		match = adrBoldMetaPattern.FindStringSubmatch(line)
	}
	if match == nil {
		return "", "", false
	}

	return strings.TrimSpace(match[1]), strings.TrimSpace(strings.TrimSuffix(match[2], "  ")), true
}

func validateADRMetadata(record adrRecord) []violation {
	required := []string{"Status", "Date", "Milestone", "Deciders", "Supersedes", "Superseded by"}
	if _, legacy := legacyADRMetadata[record.number]; legacy {
		required = required[:2]
	}

	var violations []violation
	for _, name := range required {
		if _, found := record.metadata[name]; found {
			continue
		}
		violations = append(violations, adrViolation(
			record.file,
			1,
			"adr.metadata",
			"required metadata field "+quote(name)+" is missing",
			"add `- "+name+": ...` before `## TL;DR` using the canonical ADR template",
		))
	}

	status, hasStatus := record.metadata["Status"]
	if hasStatus {
		if _, allowed := allowedADRStatuses[status.value]; !allowed {
			violations = append(violations, adrViolation(
				record.file,
				status.line,
				"adr.status",
				"status "+quote(status.value)+" is not a supported ADR lifecycle status",
				"use Proposed, Accepted, Rejected, Deprecated, or Superseded",
			))
		}
	}

	date, hasDate := record.metadata["Date"]
	if hasDate {
		parsed, err := time.Parse("2006-01-02", date.value)
		if err != nil || parsed.Format("2006-01-02") != date.value {
			violations = append(violations, adrViolation(
				record.file,
				date.line,
				"adr.date",
				"date "+quote(date.value)+" is not an ISO calendar date",
				"use an existing date in YYYY-MM-DD form",
			))
		}
	}

	milestone, hasMilestone := record.metadata["Milestone"]
	if hasMilestone && !milestonePattern.MatchString(milestone.value) {
		violations = append(violations, adrViolation(
			record.file,
			milestone.line,
			"adr.milestone",
			"milestone "+quote(milestone.value)+" does not match `MNN - Outcome`",
			"use the exact published milestone title",
		))
	}

	return violations
}

func (r *repository) readADRIndex() (map[string]adrIndexEntry, []violation) {
	document, found := r.document("docs/decisions/README.md")
	if !found {
		return nil, []violation{adrViolation(
			"docs/decisions/README.md",
			1,
			"adr.index",
			"ADR index is missing",
			"restore the canonical decision index and register every ADR",
		)}
	}

	entries := make(map[string]adrIndexEntry)
	var violations []violation
	for index, line := range document.lines {
		match := adrIndexRowPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		entry := adrIndexEntry{
			number: match[1],
			file:   strings.TrimSpace(match[2]),
			title:  strings.TrimSpace(match[3]),
			status: strings.TrimSpace(match[4]),
			date:   strings.TrimSpace(match[5]),
			line:   index + 1,
		}
		if previous, duplicate := entries[entry.number]; duplicate {
			violations = append(violations, adrViolation(
				document.path,
				entry.line,
				"adr.index",
				"ADR-"+entry.number+" duplicates index line "+strconv.Itoa(previous.line),
				"keep exactly one index row for each ADR number",
			))

			continue
		}
		entries[entry.number] = entry
	}

	return entries, violations
}

func validateADRSequence(records map[string]adrRecord) []violation {
	numbers := make([]int, 0, len(records))
	pathsByNumber := make(map[int]string, len(records))
	for number, record := range records {
		parsed, err := strconv.Atoi(number)
		if err != nil {
			continue
		}
		numbers = append(numbers, parsed)
		pathsByNumber[parsed] = record.file
	}
	sort.Ints(numbers)

	var violations []violation
	for index, number := range numbers {
		expected := index + 1
		if number == expected {
			continue
		}
		violations = append(violations, adrViolation(
			pathsByNumber[number],
			1,
			"adr.sequence",
			"ADR sequence expected "+formatADRNumber(expected)+" before "+formatADRNumber(number),
			"restore the missing historical ADR or correctly renumber unpublished ADRs and their references",
		))

		break
	}

	return violations
}

func validateADRIndex(records map[string]adrRecord, index map[string]adrIndexEntry) []violation {
	var violations []violation
	for number, record := range records {
		entry, found := index[number]
		if !found {
			violations = append(violations, adrViolation(
				record.file,
				1,
				"adr.index",
				"ADR-"+number+" is not registered in the decision index",
				"add one index row with this ADR's link, title, status, and date",
			))

			continue
		}

		expectedFile := filepath.Base(record.file)
		if entry.file != expectedFile {
			violations = append(violations, indexViolation(entry, "link points to "+quote(entry.file)+" instead of "+quote(expectedFile), "update the row link to the ADR filename"))
		}
		if record.title != "" && entry.title != record.title {
			violations = append(violations, indexViolation(entry, "decision title does not match the ADR heading", "copy the decision title from the ADR heading without rewriting accepted history"))
		}
		violations = append(violations, compareIndexMetadata(record, entry, "Status", entry.status)...)
		violations = append(violations, compareIndexMetadata(record, entry, "Date", entry.date)...)
	}
	for number, entry := range index {
		if _, found := records[number]; found {
			continue
		}
		violations = append(violations, indexViolation(entry, "index row ADR-"+number+" has no matching decision file", "restore the decision file or remove an unpublished stale index row"))
	}

	return violations
}

func compareIndexMetadata(record adrRecord, entry adrIndexEntry, field, indexValue string) []violation {
	metadata, found := record.metadata[field]
	if !found || metadata.value == indexValue {
		return nil
	}

	return []violation{indexViolation(
		entry,
		strings.ToLower(field)+" "+quote(indexValue)+" does not match ADR metadata "+quote(metadata.value),
		"make the index reflect the ADR's "+strings.ToLower(field),
	)}
}

func validateADRSupersession(records map[string]adrRecord) []violation {
	var violations []violation
	for _, record := range records {
		status := record.metadata["Status"]
		supersededBy := record.metadata["Superseded by"]
		targets := adrReferences(supersededBy.value)
		if status.value == "Superseded" && len(targets) == 0 {
			violations = append(violations, adrViolation(
				record.file,
				supersededBy.line,
				"adr.supersession",
				"a Superseded ADR does not identify its replacement",
				"set `Superseded by` to the replacement ADR number and link",
			))
		}
		if status.value != "Superseded" && len(targets) > 0 {
			violations = append(violations, adrViolation(
				record.file,
				supersededBy.line,
				"adr.supersession",
				"ADR names a replacement but its status is "+quote(status.value),
				"set Status to Superseded or remove an incorrect replacement reference",
			))
		}
		for _, target := range targets {
			replacement, found := records[target]
			if !found {
				violations = append(violations, adrViolation(
					record.file,
					supersededBy.line,
					"adr.supersession",
					"replacement ADR-"+target+" does not exist",
					"link an existing replacement ADR and register it in the index",
				))

				continue
			}
			if !containsADRReference(replacement.metadata["Supersedes"].value, record.number) {
				violations = append(violations, adrViolation(
					replacement.file,
					replacement.metadata["Supersedes"].line,
					"adr.supersession",
					"replacement does not name ADR-"+record.number+" in `Supersedes`",
					"record both sides of the supersession relationship",
				))
			}
		}
	}

	return violations
}

func adrReferences(value string) []string {
	matches := adrReferencePattern.FindAllStringSubmatch(value, -1)
	references := make([]string, 0, len(matches))
	for _, match := range matches {
		references = append(references, match[1])
	}

	return references
}

func containsADRReference(value, number string) bool {
	for _, reference := range adrReferences(value) {
		if reference == number {
			return true
		}
	}

	return false
}

func adrViolation(path string, line int, rule, message, fix string) violation {
	if line == 0 {
		line = 1
	}

	return violation{
		path:    path,
		line:    line,
		rule:    rule,
		message: message,
		fix:     fix,
		policy:  decisionPolicy,
	}
}

func indexViolation(entry adrIndexEntry, message, fix string) violation {
	return adrViolation("docs/decisions/README.md", entry.line, "adr.index", message, fix)
}

func formatADRNumber(number int) string {
	return "ADR-" + leftPad(strconv.Itoa(number), 4, '0')
}

func leftPad(value string, width int, fill byte) string {
	if len(value) >= width {
		return value
	}

	return strings.Repeat(string(fill), width-len(value)) + value
}
