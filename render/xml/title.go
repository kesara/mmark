package xml

import (
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/mmarkdown/mmark/v2/mast"
	"github.com/mmarkdown/mmark/v2/mast/reference"
)

// StatusToCategory translate the status to a category.
var StatusToCategory = map[string]string{
	"full-standard": "std",
	"standard":      "std",
	"informational": "info",
	"experimental":  "exp",
	"bcp":           "bcp",
	"historic":      "historic",
}

func (r *Renderer) titleBlock(w io.Writer, t *mast.Title) {
	// Order is fixed in RFC 7991.
	d := t.TitleData
	if d == nil {
		return
	}
	if d.SubmissionType == "" {
		d.SubmissionType = "IETF"
	}

	// rfc tag
	attrs := Attributes(
		[]string{"version", "ipr", "docName", "submissionType", "category", "xml:lang", "xmlns:xi"},
		[]string{"3", d.Ipr, t.SeriesInfo.Value, d.SubmissionType, StatusToCategory[d.SeriesInfo.Status], "en", "http://www.w3.org/2001/XInclude"},
	)
	attrs = append(attrs, Attributes(
		[]string{"updates", "obsoletes", "indexInclude"},
		[]string{IntSliceToString(d.Updates), IntSliceToString(d.Obsoletes), fmt.Sprintf("%t", d.IndexInclude)},
	)...)
	// RFC 7841 Appendix A.2.2: IETF and IRTF streams pay attention to the consensus attribute.
	// RFC 7991 Section 2.45.2: Default is false.
	if ((d.SubmissionType == "IETF") || (d.SubmissionType == "IRTF")) && d.Consensus {
		attrs = append(attrs, Attributes(
			[]string{"consensus"},
			[]string{fmt.Sprintf("%t", d.Consensus)},
		)...)
	}
	// RFC 7991 Section 2.45.11: Default is false.
	if d.SortRefs {
		attrs = append(attrs, Attributes(
			[]string{"sortRefs"},
			[]string{fmt.Sprintf("%t", d.SortRefs)},
		)...)
	}
	if t.TocDepth > 0 {
		attrs = append(attrs, Attributes(
			[]string{"tocDepth"},
			[]string{fmt.Sprintf("%d", t.TocDepth)},
		)...)
	}

	// number is deprecated, but xml2rfc want's it here to generate an actual RFC.
	// But only if number is a integer...
	if _, err := strconv.Atoi(t.SeriesInfo.Value); err == nil {
		attrs = append(attrs, Attributes(
			[]string{"number"},
			[]string{t.SeriesInfo.Value},
		)...)
	}
	r.outTag(w, "<rfc", attrs)
	r.cr(w)

	r.matter(w, &ast.DocumentMatter{Matter: ast.DocumentMatterFront})

	attrs = Attributes([]string{"abbrev"}, []string{d.Abbrev})
	r.outTag(w, "<title", attrs)
	r.outs(w, d.Title)
	r.outs(w, "</title>")

	r.titleSeriesInfo(w, d.SeriesInfo)

	for _, author := range d.Author {
		r.TitleAuthor(w, author, "author")
	}

	r.TitleDate(w, d.Date)

	r.outTagContent(w, "<area", d.Area)

	r.outTagContent(w, "<workgroup", d.Workgroup)

	r.TitleKeyword(w, d.Keyword)

	// abstract - handled by paragraph
	// note - handled by paragraph
	// boilerplate - not supported.

	return
}

// TitleAuthor outputs the author.
func (r *Renderer) TitleAuthor(w io.Writer, a mast.Author, tag string) {

	attrs := Attributes(
		[]string{"role", "initials", "surname", "fullname"},
		[]string{a.Role, a.Initials, a.Surname, a.Fullname},
	)

	r.outTag(w, "<"+tag, attrs)

	r.outTag(w, "<organization", Attributes([]string{"abbrev"}, []string{a.OrganizationAbbrev}))
	html.EscapeHTML(w, []byte(a.Organization))
	r.outs(w, "</organization>")

	r.outs(w, "<address>")
	r.outs(w, "<postal>")

	r.outTagContent(w, "<street", a.Address.Postal.Street)
	for _, street := range a.Address.Postal.Streets {
		r.outTagContent(w, "<street", street)
	}

	r.outTagMaybe(w, "<city", a.Address.Postal.City)
	for _, city := range a.Address.Postal.Cities {
		r.outTagContent(w, "<city", city)
	}

	r.outTagMaybe(w, "<cityarea", a.Address.Postal.CityArea)
	for _, city := range a.Address.Postal.CityAreas {
		r.outTagContent(w, "<cityarea", city)
	}

	r.outTagMaybe(w, "<code", a.Address.Postal.Code)
	for _, code := range a.Address.Postal.Codes {
		r.outTagContent(w, "<code", code)
	}

	r.outTagMaybe(w, "<country", a.Address.Postal.Country)
	for _, country := range a.Address.Postal.Countries {
		r.outTagContent(w, "<country", country)
	}

	r.outTagMaybe(w, "<extaddr", a.Address.Postal.ExtAddr)
	for _, extaddr := range a.Address.Postal.ExtAddrs {
		r.outTagContent(w, "<extaddr", extaddr)
	}

	r.outTagMaybe(w, "<pobox", a.Address.Postal.PoBox)
	for _, pobox := range a.Address.Postal.PoBoxes {
		r.outTagContent(w, "<pobox", pobox)
	}

	r.outTagMaybe(w, "<region", a.Address.Postal.Region)
	for _, region := range a.Address.Postal.Regions {
		r.outTagContent(w, "<region", region)
	}

	r.outs(w, "</postal>")

	r.outTagMaybe(w, "<phone", a.Address.Phone)
	r.outTagMaybe(w, "<email", a.Address.Email)
	for _, email := range a.Address.Emails {
		r.outTagContent(w, "<email", email)
	}
	r.outTagMaybe(w, "<uri", a.Address.URI)

	r.outs(w, "</address>")
	r.outs(w, "</"+tag+">")
}

// TitleDate outputs the date from the TOML title block.
func (r *Renderer) TitleDate(w io.Writer, d time.Time) {
	if d.IsZero() { // not specified
		r.outs(w, "<date/>\n")
		return
	}

	var attr = []string{}
	if x := d.Year(); x > 0 {
		attr = append(attr, fmt.Sprintf(`year="%d"`, x))
	}
	if d.Month() > 0 {
		attr = append(attr, d.Format("month=\"January\""))
	}
	if x := d.Day(); x > 0 {
		attr = append(attr, fmt.Sprintf(`day="%d"`, x))
	}
	r.outTag(w, "<date", attr)
	r.outs(w, "</date>\n")
}

// TitleKeyword outputs the keywords from the TOML title block.
func (r *Renderer) TitleKeyword(w io.Writer, keyword []string) {
	for _, k := range keyword {
		if k == "" {
			continue
		}
		r.outTagContent(w, "<keyword", k)
	}
}

// titleSeriesInfo outputs the seriesInfo from the TOML title block.
func (r *Renderer) titleSeriesInfo(w io.Writer, s reference.SeriesInfo) {
	if s.Value == "" {
		log.Printf("Empty 'value' in [seriesInfo], resulting XML may fail to parse.")
	}
	if s.Stream == "" {
		log.Printf("Empty 'stream' in [seriesInfo], resulting XML may fail to parse.")
	}
	if s.Status == "" {
		log.Printf("Empty 'status' in [seriesInfo], resulting XML may fail to parse.")
	}
	if s.Name == "" {
		log.Printf("Empty 'name' in [seriesInfo], resulting XML may fail to parse.")
	}
	attr := Attributes(
		[]string{"value", "stream", "status", "name"},
		[]string{s.Value, s.Stream, s.Status, s.Name},
	)

	r.outTag(w, "<seriesInfo", attr)
	r.outs(w, "</seriesInfo>\n")
}

// IntSliceToString converts and int slice to a string.
func IntSliceToString(is []int) string {
	if len(is) == 0 {
		return ""
	}
	s := []string{}
	for i := range is {
		s = append(s, strconv.Itoa(is[i]))
	}
	return strings.Join(s, ", ")
}

func AuthorFromTitle(fullname []byte, t *mast.Title) *mast.Author {
	if t == nil {
		return nil
	}
	full := string(fullname)
	for _, a := range t.TitleData.Author {
		if strings.EqualFold(a.Fullname, full) {
			return &a
		}
	}
	return nil
}

func ContactFromTitle(fullname []byte, t *mast.Title) *mast.Contact {
	if t == nil {
		return nil
	}
	full := string(fullname)
	for _, a := range t.TitleData.Contact {
		if strings.EqualFold(a.Fullname, full) {
			return &a
		}
	}
	return nil
}
