package mparser

import (
	"bytes"
	"encoding/xml"
	"log"
	"sort"
	"strings"

	"github.com/gomarkdown/markdown/ast"
	"github.com/mmarkdown/mmark/v2/mast"
	"github.com/mmarkdown/mmark/v2/mast/reference"
)

// CitationToBibliography walks the AST and gets all the citations from HTML blocks and groups them into
// normative and informative references.
func CitationToBibliography(doc ast.Node) (normative ast.Node, informative ast.Node) {
	seen := map[string]*mast.BibliographyItem{}
	raw := map[string][]byte{}
	names := []string{} // names of the authors and contacts

	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if t, ok := node.(*mast.Title); ok {
			names = authContFromTitle(t)
			return ast.Terminate
		}
		return ast.GoToNext
	})

	// Gather all citations, but check for contacts/author citation, as we want to exclude
	// those here - otherwise they end up in the bibliography.
	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		switch c := node.(type) {
		case *ast.Citation:
		Destination:
			for i, d := range c.Destination {
				for n := range names {
					if strings.EqualFold(names[n], string(d)) {
						// author/contact ref -> exclude
						continue Destination
					}

				}
				if _, ok := seen[string(bytes.ToLower(d))]; ok {
					continue Destination
				}
				ref := &mast.BibliographyItem{}
				ref.Anchor = d
				ref.Type = c.Type[i]

				seen[string(d)] = ref
			}
		case *mast.ReferenceBlock:
			anchor := anchorFromReference(c.Literal)
			if anchor != nil {
				raw[string(bytes.ToLower(anchor))] = c.Literal
			}
		}
		return ast.GoToNext
	})

	// sort on anchor, so it is stable when outputting the bibliography.
	keys := make([]string, len(seen))
	i := 0
	for k := range seen {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	for _, k := range keys {
		r := seen[k]
		// If we have a reference anchor and the raw XML add that here.
		if rw, ok := raw[string(bytes.ToLower(r.Anchor))]; ok {
			var x reference.Reference
			if e := xml.Unmarshal(rw, &x); e != nil {
				log.Printf("Failed to unmarshal reference: %q: %s, assuming <referencegroup>", r.Anchor, e)
				r.ReferenceGroup = rw
			} else {
				r.Reference = &x
			}
		}

		switch r.Type {
		case ast.CitationTypeSuppressed:
			fallthrough
		case ast.CitationTypeInformative:
			if informative == nil {
				informative = &mast.Bibliography{Type: ast.CitationTypeInformative}
			}

			ast.AppendChild(informative, r)
		case ast.CitationTypeNormative:
			if normative == nil {
				normative = &mast.Bibliography{Type: ast.CitationTypeNormative}
			}
			ast.AppendChild(normative, r)
		}
	}
	return normative, informative
}

// NodeBackMatter is the place where we should inject the bibliography
func NodeBackMatter(doc ast.Node) ast.Node {
	var matter ast.Node

	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if mat, ok := node.(*ast.DocumentMatter); ok {
			if mat.Matter == ast.DocumentMatterBack {
				matter = mat
				return ast.Terminate
			}
		}
		return ast.GoToNext
	})
	return matter
}

// Parse '<reference anchor='CBR03' target=">' and return the string after anchor= is the ID for the reference.
func anchorFromReference(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte("<reference ")) && !bytes.HasPrefix(data, []byte("<referencegroup ")) {
		return nil
	}

	anchor := bytes.Index(data, []byte("anchor="))
	if anchor < 0 {
		return nil
	}

	beg := anchor + 7
	if beg >= len(data) {
		return nil
	}

	quote := data[beg]

	i := beg + 1
	// scan for an end-of-reference marker
	for i < len(data) && data[i] != quote {
		i++
	}
	// no end-of-reference marker
	if i >= len(data) {
		return nil
	}
	return data[beg+1 : i]
}

// ReferenceHook is the hook used to parse reference nodes.
func ReferenceHook(data []byte) (ast.Node, []byte, int) {
	ref, ok := IsReference(data)
	if !ok {
		return nil, nil, 0
	}

	node := &mast.ReferenceBlock{}
	node.Literal = fmtReference(ref)
	return node, nil, len(ref)
}

// IsReference returns wether data contains a reference.
func IsReference(data []byte) ([]byte, bool) {
	typ := ""
	if bytes.HasPrefix(data, []byte("<reference ")) {
		typ = "</reference>"
	}
	if bytes.HasPrefix(data, []byte("<referencegroup ")) {
		typ = "</referencegroup>"
	}
	if typ == "" {
		return nil, false
	}

	// scan for an end-of-reference marker, across lines if necessary
	end := bytes.Index(data[len(typ):], []byte(typ))
	if end > len(data) || end == 0 {
		return nil, false
	}

	return data[:end+2*len(typ)], true
}

func fmtReference(data []byte) []byte {
	var x reference.Reference
	if e := xml.Unmarshal(data, &x); e != nil {
		return data
	}

	out, e := xml.MarshalIndent(x, "", "   ")
	if e != nil {
		return data
	}
	return out
}

// AddBibliography adds the bibliography to the document. It will be
// added just after the backmatter node. If that node can't be found this
// function returns false and does nothing.
func AddBibliography(doc ast.Node) bool {
	norm, inform := CitationToBibliography(doc)
	where := NodeBackMatter(doc)
	if where == nil {
		if norm != nil || inform != nil {
			log.Print("No {backmatter} found, can't insert bibliography")
		}
		return false
	}

	// RFC 7322 Section 4.8.6: When both normative and informative references
	// exist, the references section should be split into two subsections.
	if (norm != nil) && (inform != nil) {
		wrapper := &mast.BibliographyWrapper{}
		ast.AppendChild(where, wrapper)
		where = wrapper
	}

	if norm != nil {
		ast.AppendChild(where, norm)
	}
	if inform != nil {
		ast.AppendChild(where, inform)
	}
	return (norm != nil) || (inform != nil)
}

// Authors FromTitle returns all the authors or contacts from the title block.
func authContFromTitle(t *mast.Title) []string {
	if t == nil {
		return nil
	}
	names := []string{}
	for _, a := range t.TitleData.Author {
		names = append(names, a.Fullname)
	}
	for _, c := range t.TitleData.Contact {
		names = append(names, c.Fullname)
	}
	return names
}
