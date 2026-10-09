package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/html"
)

var tokenPattern = regexp.MustCompile(`[\pL\pN_]+(?:['.:-][\pL\pN_]+)*|[^\s\pL\pN_\x{200b}]`)

// Tokens is an experimental reconstruction, not an official JW Library tokenizer.
func Tokens(text string) []string {
	return tokenPattern.FindAllString(strings.Join(strings.Fields(text), " "), -1)
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func ParagraphText(src string, pid int) (string, error) {
	n, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return "", err
	}
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if attr(n, "data-pid") == strconv.Itoa(pid) {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	if found == nil {
		return "", fmt.Errorf("paragraph pid %d not found", pid)
	}
	var text strings.Builder
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
			return
		}
		for _, class := range strings.Fields(attr(n, "class")) {
			switch class {
			case "parNum", "pageNum", "fn", "footnoteLink":
				return
			}
		}
		if n.Data == "rt" || n.Data == "textarea" {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(found)
	return strings.Join(strings.Fields(text.String()), " "), nil
}

type Highlight struct {
	DocID int64  `json:"docid"`
	PID   int    `json:"pid"`
	Quote string `json:"quote"`
	Color int    `json:"color"`
}
type AnnotationNote struct {
	Highlight int    `json:"highlight"`
	Title     string `json:"title"`
	Content   string `json:"content"`
}
type Answer struct {
	DocID   int64  `json:"docid"`
	TextTag string `json:"text_tag"`
	Value   string `json:"value"`
}
type AnnotationPlan struct {
	Highlights []Highlight     `json:"highlights"`
	Note       *AnnotationNote `json:"note"`
	Answers    []Answer        `json:"answers"`
}
type AnnotationRange struct {
	DocID int64  `json:"docid"`
	PID   int    `json:"pid"`
	Start int    `json:"start_token"`
	End   int    `json:"end_token"`
	Guid  string `json:"guid"`
}
type AnnotationReport struct {
	Ranges       []AnnotationRange `json:"ranges"`
	Notes        int               `json:"notes_added"`
	Answers      int               `json:"answers_added"`
	Experimental bool              `json:"experimental_tokenizer"`
}

func quoteRange(text, quote string) (int, int, error) {
	t, q := Tokens(text), Tokens(quote)
	if len(q) == 0 {
		return 0, 0, errors.New("highlight quote is empty")
	}
	start := -1
	for i := 0; i+len(q) <= len(t); i++ {
		if strings.Join(t[i:i+len(q)], "\x00") == strings.Join(q, "\x00") {
			if start != -1 {
				return 0, 0, errors.New("highlight quote is ambiguous")
			}
			start = i
		}
	}
	if start < 0 {
		return 0, 0, errors.New("highlight quote is not in the paragraph")
	}
	return start, start + len(q) - 1, nil
}

func document(ctx context.Context, library *sql.DB, docid int64) (row, string, error) {
	v := row{"LocationId": nil, "BookNumber": nil, "ChapterNumber": nil, "DocumentId": docid, "Track": nil, "Type": int64(0), "Specialty": nil, "Edition": nil}
	var symbol, title, src string
	var language, issue int64
	err := library.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(pub.meps_symbol,''),pub.symbol),pub.meps_lang,COALESCE(pub.issue_tag,0),COALESCE(doc.title,''),doc.html FROM doc JOIN pub ON pub.id=doc.pub_id WHERE doc.docid=?`, docid).Scan(&symbol, &language, &issue, &title, &src)
	if err != nil {
		return nil, "", fmt.Errorf("document %d in local library: %w", docid, err)
	}
	v["KeySymbol"] = symbol
	v["MepsLanguage"] = language
	v["IssueTagNumber"] = issue
	v["Title"] = title
	return v, src, nil
}

func annotationLocation(ctx context.Context, tx *sql.Tx, v row) (int64, error) {
	r, err := tx.QueryContext(ctx, "SELECT * FROM Location")
	if err != nil {
		return 0, err
	}
	cols, err := r.Columns()
	if err != nil {
		r.Close()
		return 0, err
	}
	var maxID int64
	for r.Next() {
		values := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range values {
			ptr[i] = &values[i]
		}
		if err = r.Scan(ptr...); err != nil {
			r.Close()
			return 0, err
		}
		held := row{}
		for i, c := range cols {
			held[c] = values[i]
		}
		id := held["LocationId"].(int64)
		if id > maxID {
			maxID = id
		}
		if locationKey(held) == locationKey(v) {
			r.Close()
			return id, nil
		}
	}
	err = r.Err()
	r.Close()
	if err != nil {
		return 0, err
	}
	v["LocationId"] = maxID + 1
	if err = insert(ctx, tx, "Location", v); err != nil {
		return 0, err
	}
	return maxID + 1, nil
}

func Annotate(ctx context.Context, input, output string, library *sql.DB, plan AnnotationPlan, device string) (*AnnotationReport, error) {
	if len(plan.Highlights) == 0 && plan.Note == nil && len(plan.Answers) == 0 {
		return nil, errors.New("annotation plan is empty")
	}
	a, err := Open(ctx, input)
	if err != nil {
		return nil, err
	}
	defer a.Close()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	r := &AnnotationReport{Experimental: true, Ranges: []AnnotationRange{}}
	var markIDs, locationIDs []int64
	for _, h := range plan.Highlights {
		if h.Color < 1 || h.Color > 6 {
			return nil, errors.New("highlight color must be between 1 and 6")
		}
		v, src, e := document(ctx, library, h.DocID)
		if e != nil {
			return nil, e
		}
		text, e := ParagraphText(src, h.PID)
		if e != nil {
			return nil, e
		}
		start, end, e := quoteRange(text, h.Quote)
		if e != nil {
			return nil, e
		}
		loc, e := annotationLocation(ctx, tx, v)
		if e != nil {
			return nil, e
		}
		var overlaps int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM BlockRange b JOIN UserMark u USING(UserMarkId) WHERE u.LocationId=? AND b.BlockType=1 AND b.Identifier=? AND (b.StartToken IS NULL OR b.EndToken IS NULL OR (b.StartToken<=? AND b.EndToken>=?))`, loc, h.PID, end, start).Scan(&overlaps); e != nil {
			return nil, e
		}
		if overlaps != 0 {
			return nil, errors.New("annotation overlaps an existing highlight")
		}
		guid := uuid.NewString()
		res, e := tx.ExecContext(ctx, `INSERT INTO UserMark(ColorIndex,LocationId,StyleIndex,UserMarkGuid,Version) VALUES(?,?,0,?,1)`, h.Color, loc, guid)
		if e != nil {
			return nil, e
		}
		id, e := res.LastInsertId()
		if e != nil {
			return nil, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO BlockRange(BlockType,Identifier,StartToken,EndToken,UserMarkId) VALUES(1,?,?,?,?)`, h.PID, start, end, id); e != nil {
			return nil, e
		}
		markIDs = append(markIDs, id)
		locationIDs = append(locationIDs, loc)
		r.Ranges = append(r.Ranges, AnnotationRange{h.DocID, h.PID, start, end, guid})
	}
	if n := plan.Note; n != nil {
		if n.Highlight < 0 || n.Highlight >= len(markIDs) || strings.TrimSpace(n.Content) == "" {
			return nil, errors.New("note requires nonempty content and a valid highlight index")
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err = tx.ExecContext(ctx, `INSERT INTO Note(Guid,UserMarkId,LocationId,Title,Content,LastModified,Created,BlockType,BlockIdentifier) VALUES(?,?,?,?,?,?,?,1,?)`, uuid.NewString(), markIDs[n.Highlight], locationIDs[n.Highlight], n.Title, n.Content, now, now, plan.Highlights[n.Highlight].PID); err != nil {
			return nil, err
		}
		r.Notes++
	}
	for _, answer := range plan.Answers {
		v, src, e := document(ctx, library, answer.DocID)
		if e != nil {
			return nil, e
		}
		if !validTextTag(src, answer.TextTag) {
			return nil, fmt.Errorf("%q is not a textarea id in document %d", answer.TextTag, answer.DocID)
		}
		if strings.TrimSpace(answer.Value) == "" {
			return nil, errors.New("answer is empty")
		}
		loc, e := annotationLocation(ctx, tx, v)
		if e != nil {
			return nil, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO InputField(LocationId,TextTag,Value) VALUES(?,?,?)", loc, answer.TextTag, answer.Value); e != nil {
			return nil, fmt.Errorf("answer already exists or is invalid: %w", e)
		}
		r.Answers++
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if device == "" {
		device = a.Manifest.Backup.Device + " (annotation prototype)"
	}
	if err = a.Save(ctx, output, device); err != nil {
		return nil, err
	}
	return r, nil
}

func validTextTag(src, tag string) bool {
	if !regexp.MustCompile(`^tt[0-9]+$`).MatchString(tag) {
		return false
	}
	n, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return false
	}
	found := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Data == "textarea" && attr(n, "id") == tag {
			found = true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}
