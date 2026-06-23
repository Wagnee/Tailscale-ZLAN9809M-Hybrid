package uci

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

type Section struct {
	Type    string
	Name    string
	Options map[string]string
}

func Parse(path string) ([]Section, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseReader(path, f)
}

func ParseReader(name string, reader io.Reader) ([]Section, error) {
	var sections []Section
	var current *Section
	scanner := bufio.NewScanner(reader)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		words, err := splitWords(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, lineNumber, err)
		}
		if len(words) == 0 {
			continue
		}

		switch words[0] {
		case "config":
			if len(words) < 2 {
				return nil, fmt.Errorf("%s:%d: secao config invalida", name, lineNumber)
			}
			sectionName := ""
			if len(words) > 2 {
				sectionName = words[2]
			}
			sections = append(sections, Section{
				Type: words[1], Name: sectionName, Options: make(map[string]string),
			})
			current = &sections[len(sections)-1]
		case "option":
			if current == nil || len(words) < 3 {
				return nil, fmt.Errorf("%s:%d: option fora de uma secao", name, lineNumber)
			}
			current.Options[words[1]] = words[2]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return sections, nil
}

func splitWords(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped := false
	inWord := false

	flush := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}

	for _, r := range line {
		if escaped {
			word.WriteRune(r)
			inWord = true
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			inWord = true
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
				inWord = true
			}
			continue
		}
		if r == '\'' || r == '"' {
			inWord = true
			quote = r
			continue
		}
		if r == '#' {
			break
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		word.WriteRune(r)
		inWord = true
	}

	if escaped || quote != 0 {
		return nil, fmt.Errorf("aspas ou escape nao finalizado")
	}
	flush()
	return words, nil
}
