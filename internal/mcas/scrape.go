package mcas

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var (
	hiddenInputRE = regexp.MustCompile(`(?i)<input[^>]*type="hidden"[^>]*>`)
	nameAttrRE    = regexp.MustCompile(`(?i)name="([^"]+)"`)
	valueAttrRE   = regexp.MustCompile(`(?i)value="([^"]*)"`)
	studentNameRE = regexp.MustCompile(`(?i)id="[^"]*StudentName[^"]*"[^>]*>([^<]{2,80})`)
	masterVarRE   = regexp.MustCompile(`var\s+(master_[A-Za-z]+)\s*=\s*"?([^";\n]{0,80})`)
	dayHeaderRE   = regexp.MustCompile(`([A-Z][a-z]+)\s*(\d{1,2})(?:st|nd|rd|th)?\s*([A-Z][a-z]{2})`)
	balanceRE     = regexp.MustCompile(`£\s*(-?[\d,]+\.\d{2})`)
)

// hiddenFields scrapes every <input type="hidden"> field from a WebForms
// page. Matching on type="hidden" rather than hardcoding field names picks
// up postback plumbing like __VIEWSTATE / __EVENTVALIDATION without needing
// to track ASP.NET's internal field names, so it survives markup changes.
func hiddenFields(pageHTML string) map[string]string {
	out := map[string]string{}
	for _, tag := range hiddenInputRE.FindAllString(pageHTML, -1) {
		name := nameAttrRE.FindStringSubmatch(tag)
		if name == nil {
			continue
		}
		value := valueAttrRE.FindStringSubmatch(tag)
		if value != nil {
			out[name[1]] = value[1]
		} else {
			out[name[1]] = ""
		}
	}
	return out
}

// dashboardContext is the pupil identity captured off the dashboard page
// after a successful login.
type dashboardContext struct {
	StudentID   int
	SchoolID    int
	SchoolName  string
	StudentName string
}

// readDashboardContext pulls ids and the pupil name off the dashboard HTML.
// The page defines master_studentid / master_schoolID as plain JS vars,
// which is cheaper and more reliable than deriving them from the API, and
// it's the only place the pupil's name appears at all.
func readDashboardContext(pageHTML string) dashboardContext {
	var ctx dashboardContext
	for _, m := range masterVarRE.FindAllStringSubmatch(pageHTML, -1) {
		switch m[1] {
		case "master_studentid":
			if n, err := strconv.Atoi(strings.TrimSpace(m[2])); err == nil {
				ctx.StudentID = n
			}
		case "master_schoolID":
			if n, err := strconv.Atoi(strings.TrimSpace(m[2])); err == nil {
				ctx.SchoolID = n
			}
		case "master_schoolName":
			ctx.SchoolName = m[2]
		}
	}
	if m := studentNameRE.FindStringSubmatch(pageHTML); m != nil {
		name := strings.TrimSpace(m[1])
		// Rendered "Surname, Forename" in the header.
		if surname, forename, ok := strings.Cut(name, ","); ok {
			name = strings.TrimSpace(forename) + " " + strings.TrimSpace(surname)
		}
		ctx.StudentName = name
	}
	return ctx
}

// findErrorText walks the login page looking for validation text MCAS
// rendered inline, matching the elements the reference repo scrapes:
// [id*=Error], [id*=Message], [id*=Validation], .alert.
func findErrorText(pageHTML string) string {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return ""
	}
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode {
			id, class := attr(n, "id"), attr(n, "class")
			matches := containsFold(id, "error") || containsFold(id, "message") ||
				containsFold(id, "validation") || containsFold(class, "alert")
			if matches {
				text := strings.TrimSpace(textContent(n))
				if text != "" && len(text) < 200 {
					found = text
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func containsFold(haystack, needle string) bool {
	return haystack != "" && strings.Contains(strings.ToLower(haystack), needle)
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(sb.String()), " ")
}

// parseDayHeader turns a timetable column header into a day name and, if
// resolvable, an ISO date. "Monday14th Sep" becomes ("Monday", 2026-09-14).
// The header carries no year, so the nearest one within six months of today
// is assumed.
func parseDayHeader(header string, today time.Time) (string, *time.Time) {
	m := dayHeaderRE.FindStringSubmatch(header)
	if m == nil {
		return header, nil
	}
	name, day, month := m[1], m[2], m[3]
	for _, year := range []int{today.Year(), today.Year() + 1, today.Year() - 1} {
		parsed, err := time.Parse("2 Jan 2006", day+" "+month+" "+strconv.Itoa(year))
		if err != nil {
			continue
		}
		diff := parsed.Sub(today).Hours() / 24
		if diff < 0 {
			diff = -diff
		}
		if diff <= 180 {
			return name, &parsed
		}
	}
	return name, nil
}

// parseBehaviourHTML reads the per-day behaviour events table. MCAS returns
// this view as an HTML fragment rather than JSON, and carries free-text
// columns (e.g. a human-readable event level) that the year-wide JSON call
// doesn't expose, so rows are kept as loosely-typed maps rather than a
// fixed struct.
func parseBehaviourHTML(fragment string) []map[string]string {
	if strings.TrimSpace(fragment) == "" {
		return nil
	}
	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil
	}
	var headers []string
	var rows [][]string
	var walk func(*html.Node, bool)
	inHead := false
	walk = func(n *html.Node, inBody bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "thead":
				inHead = true
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, inBody)
				}
				inHead = false
				return
			case "th":
				if inHead {
					headers = append(headers, textContent(n))
				}
			case "tr":
				if !inHead {
					var cells []string
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.ElementNode && c.Data == "td" {
							cells = append(cells, textContent(c))
						}
					}
					if len(cells) > 0 {
						rows = append(rows, cells)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inBody)
		}
	}
	walk(doc, false)

	events := make([]map[string]string, 0, len(rows))
	for _, cells := range rows {
		row := map[string]string{}
		if len(headers) > 0 {
			for i, cell := range cells {
				if i < len(headers) {
					row[headers[i]] = cell
				}
			}
		} else {
			row["Event"] = cells[len(cells)-1]
		}
		events = append(events, row)
	}
	return events
}

// parseTimetableHTML reads the rendered weekly timetable grid. There is no
// API route for this - MCSTimetable.aspx is server-rendered - so the grid is
// read from the page. Each cell is a stack of divs (period, school, subject,
// class, teacher) whose title attributes hold the untruncated text, which is
// what's used; the visible text is ellipsised.
func parseTimetableHTML(pageHTML string, today time.Time) []Lesson {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil
	}
	var lessons []Lesson
	var findTable func(*html.Node) *html.Node
	findTable = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && n.Data == "table" {
			var headers []string
			collectText(n, "th", &headers)
			joined := strings.Join(headers, " ")
			if len(headers) > 0 && (strings.Contains(joined, "Monday") || strings.Contains(joined, "Tuesday")) {
				return n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := findTable(c); found != nil {
				return found
			}
		}
		return nil
	}
	table := findTable(doc)
	if table == nil {
		return nil
	}

	var headerTexts []string
	collectText(table, "th", &headerTexts)
	type dayCol struct {
		name string
		date *time.Time
	}
	days := make([]dayCol, len(headerTexts))
	for i, h := range headerTexts {
		name, date := parseDayHeader(h, today)
		days[i] = dayCol{name: name, date: date}
	}

	var rows []*html.Node
	collectNodes(table, "tr", &rows)
	if len(rows) > 1 {
		rows = rows[1:] // skip the header row
	} else {
		rows = nil
	}
	for _, row := range rows {
		var cells []*html.Node
		for c := row.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "td" {
				cells = append(cells, c)
			}
		}
		for index, cell := range cells {
			if index >= len(days) {
				break
			}
			var divs []*html.Node
			collectNodes(cell, "div", &divs)
			if len(divs) < 3 {
				continue
			}
			values := make([]string, len(divs))
			for i, d := range divs {
				if t := attr(d, "title"); t != "" {
					values[i] = t
				} else {
					values[i] = textContent(d)
				}
			}
			period, subject := values[0], values[2]
			if subject == "" {
				continue
			}
			lesson := Lesson{
				Day:     days[index].name,
				Date:    days[index].date,
				Period:  period,
				Subject: subject,
			}
			if len(values) > 3 {
				lesson.Class = values[3]
			}
			if len(values) > 4 {
				lesson.Teacher = values[4]
			}
			lessons = append(lessons, lesson)
		}
	}
	return lessons
}

func collectText(n *html.Node, tag string, out *[]string) {
	if n.Type == html.ElementNode && n.Data == tag {
		*out = append(*out, textContent(n))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, tag, out)
	}
}

func collectNodes(n *html.Node, tag string, out *[]*html.Node) {
	if n.Type == html.ElementNode && n.Data == tag {
		*out = append(*out, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectNodes(c, tag, out)
	}
}

// parseDinnerBalance scrapes the credit balance from the dashboard widget's
// HTML fragment, e.g. "Credit Balance Summary : £ 12.34".
func parseDinnerBalance(fragment string) (float64, bool) {
	m := balanceRE.FindStringSubmatch(fragment)
	if m == nil {
		return 0, false
	}
	cleaned := strings.ReplaceAll(m[1], ",", "")
	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, false
	}
	return amount, true
}

// sortEventsNewestFirst orders behaviour events most-recent-first.
func sortEventsNewestFirst(events []BehaviourEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Date.After(events[j].Date)
	})
}
