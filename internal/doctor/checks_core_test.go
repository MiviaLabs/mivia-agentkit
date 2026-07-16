// Package doctor validates installed mivia-agent control surfaces.
// Plan: WS3. PRD: FR-2.1, FR-10.5.
package doctor

import "testing"

func TestValidSkillFrontmatterAcceptsCRLFLineEndings(t *testing.T) {
	// git's core.autocrlf (the GitHub Actions Windows runner default)
	// checks text files out with \r\n line endings.
	data := []byte("---\r\nname: example\r\ndescription: does a thing\r\n---\r\nbody\r\n")
	if !validSkillFrontmatter(data) {
		t.Fatal("validSkillFrontmatter() = false, want true for CRLF frontmatter")
	}
}

func TestValidSkillFrontmatterAcceptsLFLineEndings(t *testing.T) {
	data := []byte("---\nname: example\ndescription: does a thing\n---\nbody\n")
	if !validSkillFrontmatter(data) {
		t.Fatal("validSkillFrontmatter() = false, want true for LF frontmatter")
	}
}

func TestValidSkillFrontmatterRejectsMissingFields(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("---\nname: example\n---\nbody\n"),
		[]byte("no frontmatter at all\n"),
		[]byte("---\nname: example\ndescription:\n---\nbody\n"),
	} {
		if validSkillFrontmatter(data) {
			t.Fatalf("validSkillFrontmatter(%q) = true, want false", data)
		}
	}
}
