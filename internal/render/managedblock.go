// Package render renders embedded templates into target repo files.
// Plan: WS2. PRD: FR-1.2, FR-1.3.
package render

import (
	"bytes"
	"errors"
)

var markerPairs = [][2][]byte{
	{[]byte("<!-- mivia-agent:managed:start -->"), []byte("<!-- mivia-agent:managed:end -->")},
	{[]byte("# mivia-agent:managed:start"), []byte("# mivia-agent:managed:end")},
}

// ExtractManaged splits content around a managed block.
func ExtractManaged(content []byte) (pre, managed, post []byte, ok bool) {
	for _, pair := range markerPairs {
		start := bytes.Index(content, pair[0])
		if start < 0 {
			continue
		}
		afterStart := start + len(pair[0])
		endRel := bytes.Index(content[afterStart:], pair[1])
		if endRel < 0 {
			return nil, nil, nil, false
		}
		end := afterStart + endRel
		postStart := end + len(pair[1])
		return content[:start], content[afterStart:end], content[postStart:], true
	}
	return nil, nil, nil, false
}

// HasManaged reports whether content has a complete managed block.
func HasManaged(content []byte) bool {
	_, _, _, ok := ExtractManaged(content)
	return ok
}

// ReplaceManaged preserves user content and replaces only the managed section.
func ReplaceManaged(original, newManaged []byte) ([]byte, error) {
	for _, pair := range markerPairs {
		start := bytes.Index(original, pair[0])
		if start < 0 {
			continue
		}
		afterStart := start + len(pair[0])
		endRel := bytes.Index(original[afterStart:], pair[1])
		if endRel < 0 {
			return nil, errors.New("managed block start without end")
		}
		end := afterStart + endRel
		var out bytes.Buffer
		out.Write(original[:afterStart])
		out.Write(newManaged)
		out.Write(original[end:])
		return out.Bytes(), nil
	}
	for _, pair := range markerPairs {
		if bytes.Contains(original, pair[1]) {
			return nil, errors.New("managed block end without start")
		}
	}
	return nil, errors.New("managed block not found")
}
