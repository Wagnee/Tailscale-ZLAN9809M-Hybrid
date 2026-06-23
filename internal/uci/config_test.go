package uci

import (
	"strings"
	"testing"
)

func TestParseReader(t *testing.T) {
	input := `
# comentario
config main 'main'
    option enabled '1'
    option auth_key ''
    option label "PLC Principal"
    option escaped 'valor com espacos'
`
	sections, err := ParseReader("test", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 {
		t.Fatalf("len = %d, want 1", len(sections))
	}
	s := sections[0]
	if s.Type != "main" || s.Name != "main" {
		t.Fatalf("section = %#v", s)
	}
	for key, want := range map[string]string{
		"enabled": "1", "auth_key": "", "label": "PLC Principal", "escaped": "valor com espacos",
	} {
		if got := s.Options[key]; got != want {
			t.Errorf("option %s = %q, want %q", key, got, want)
		}
	}
}

func TestParseReaderRejectsUnclosedQuote(t *testing.T) {
	_, err := ParseReader("test", strings.NewReader("config main 'broken\n"))
	if err == nil {
		t.Fatal("expected an error")
	}
}
