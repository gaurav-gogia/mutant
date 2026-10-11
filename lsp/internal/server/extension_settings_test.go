package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheExtensionNeverWritesTheUsersSettings is M26-LSP-018. On activation,
// and again whenever mutant.format.onType.enabled changed, the extension called
// WorkspaceConfiguration.update("formatOnType", ..., ConfigurationTarget.Workspace).
// That created .vscode/settings.json in whatever folder was open -- a case
// folder as readily as a project -- and, unless that file already had a
// [mutant] block, wrote the plain editor.formatOnType key: format-on-type went
// on or off for every other language, while the [mutant] default of false kept
// it off for Mutant whatever the setting said. The extension now writes
// nothing. Its manifest turns editor.formatOnType on for Mutant, so that VS Code
// asks for on-type edits at all, and a languageclient middleware passes the
// request to the server only while mutant.format.onType.enabled is on.
//
// The extension's own tests need VS Code and are run by hand, so this reads its
// source and manifest, as the lint-rule tests in lint_config_test.go do.
func TestTheExtensionNeverWritesTheUsersSettings(t *testing.T) {
	root := filepath.Join("..", "..", "..", "mutant-vscode-extension")
	source, err := os.ReadFile(filepath.Join(root, "src", "extension.ts"))
	if err != nil {
		t.Skipf("extension source not readable here: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Skipf("extension manifest not readable here: %v", err)
	}
	for _, problem := range extensionSettingsProblems(source, manifest) {
		t.Error(problem)
	}
}

// extensionSettingsProblems lists what is wrong, for M26-LSP-018, with the
// extension whose source and manifest are given: each sign that it writes the
// user's settings, and each reason the on-type opt-in it offers could not take
// effect without such a write.
func extensionSettingsProblems(source, manifest []byte) []string {
	var problems []string
	code, shape := typeScriptViews(string(source))

	// WorkspaceConfiguration.update is how an extension writes a setting, and
	// ConfigurationTarget names the settings file the write lands in.
	for _, write := range []string{".update(", "ConfigurationTarget"} {
		if strings.Contains(code, write) {
			problems = append(problems, fmt.Sprintf("extension.ts uses %s, which writes configuration; "+
				"the extension must not write into the user's settings files", write))
		}
	}

	var parsed struct {
		Contributes struct {
			Configuration struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"configuration"`
			ConfigurationDefaults map[string]json.RawMessage `json:"configurationDefaults"`
		} `json:"contributes"`
	}
	if err := json.Unmarshal(manifest, &parsed); err != nil {
		return append(problems, fmt.Sprintf("the extension manifest does not parse: %v", err))
	}
	if _, offered := parsed.Contributes.Configuration.Properties["mutant.format.onType.enabled"]; !offered {
		// No opt-in, so there is nothing to gate: VS Code's own [mutant]
		// editor.formatOnType is then the only switch.
		return problems
	}

	// VS Code asks an on-type provider only while editor.formatOnType is on for
	// the document's language. With the [mutant] default off, the opt-in could
	// take effect only by writing that setting, which is the defect.
	var mutantDefaults map[string]any
	if raw, ok := parsed.Contributes.ConfigurationDefaults["[mutant]"]; ok {
		if err := json.Unmarshal(raw, &mutantDefaults); err != nil {
			return append(problems, fmt.Sprintf("configurationDefaults[\"[mutant]\"] does not parse: %v", err))
		}
	}
	if value, set := mutantDefaults["editor.formatOnType"]; value != true {
		shown := fmt.Sprint(value)
		if !set {
			shown = "VS Code's own default, false"
		}
		problems = append(problems, fmt.Sprintf("mutant.format.onType.enabled is offered but [mutant] "+
			"editor.formatOnType defaults to %s: VS Code never asks an on-type provider while that is off, "+
			"so the setting could take effect only by writing the user's settings", shown))
	}

	// With editor.formatOnType on, the setting is the whole switch, and the
	// languageclient middleware is where it acts: a handler that does not call
	// next sends no request. So the handler has to read the setting, itself or
	// through a function extension.ts declares, and has to call next.
	hook := regexp.MustCompile(`\bprovideOnTypeFormattingEdits\b`).FindStringIndex(code)
	if hook == nil {
		return append(problems, "mutant.format.onType.enabled is offered but extension.ts has no "+
			"provideOnTypeFormattingEdits middleware, so nothing decides by it whether the server is asked")
	}
	handler := code[hook[0]:propertyEnd(shape, hook[1])]
	if !strings.Contains(handler, "next(") {
		problems = append(problems, "the provideOnTypeFormattingEdits middleware never calls next, so the "+
			"server is not asked even with mutant.format.onType.enabled on")
	}
	reads := readsOnTypeSetting(handler)
	for _, call := range regexp.MustCompile(`\b([A-Za-z_]\w*)\s*\(`).FindAllStringSubmatch(handler, -1) {
		reads = reads || readsOnTypeSetting(declarationOf(code, shape, call[1]))
	}
	if !reads {
		problems = append(problems, "the provideOnTypeFormattingEdits middleware does not read "+
			"mutant.format.onType.enabled, itself or through a function extension.ts declares, so the "+
			"setting does not decide whether the server is asked")
	}
	return problems
}

// readsOnTypeSetting reports whether text reads mutant.format.onType.enabled:
// by its whole key, or as format.onType.enabled of the mutant section.
func readsOnTypeSetting(text string) bool {
	wholeKey := regexp.MustCompile(`["']mutant\.format\.onType\.enabled["']`)
	section := regexp.MustCompile(`getConfiguration\(\s*["']mutant["']`)
	key := regexp.MustCompile(`["']format\.onType\.enabled["']`)
	return wholeKey.MatchString(text) || (section.MatchString(text) && key.MatchString(text))
}

// declarationOf is the text of the function code declares as name, or "" when
// it declares none: `function name(...) {...}` through its closing brace, or
// `const name = ...` through the end of the statement.
func declarationOf(code, shape, name string) string {
	quoted := regexp.QuoteMeta(name)
	if loc := regexp.MustCompile(`\bfunction\s+` + quoted + `\s*\(`).FindStringIndex(code); loc != nil {
		params := matchingBracket(shape, loc[1]-1)
		if params < 0 {
			return ""
		}
		open := strings.IndexByte(shape[params:], '{')
		if open < 0 {
			return ""
		}
		end := matchingBracket(shape, params+open)
		if end < 0 {
			return ""
		}
		return code[loc[0] : end+1]
	}
	if loc := regexp.MustCompile(`\b(?:const|let|var)\s+` + quoted + `\b`).FindStringIndex(code); loc != nil {
		return code[loc[0]:propertyEnd(shape, loc[1])]
	}
	return ""
}

// propertyEnd is where the property or declaration whose name ends at from
// ends in shape: at the first comma or semicolon outside brackets, or at the
// bracket that closes the object or call it is in.
func propertyEnd(shape string, from int) int {
	depth := 0
	for i := from; i < len(shape); i++ {
		switch shape[i] {
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			if depth == 0 {
				return i
			}
			depth--
		case ',', ';':
			if depth == 0 {
				return i
			}
		}
	}
	return len(shape)
}

// matchingBracket is the index of the bracket in shape that closes the one at
// open, or -1 when nothing does.
func matchingBracket(shape string, open int) int {
	depth := 0
	for i := open; i < len(shape); i++ {
		switch shape[i] {
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

// typeScriptViews returns two copies of TypeScript source that keep its byte
// offsets. In code every comment is blanked, so a search of it reads only what
// runs: a comment that mentions .update( is not a write. In shape the contents
// of string and template literals are blanked as well, so brackets can be
// matched in it by counting. A regular-expression literal is not told apart
// from a division; extension.ts has none.
func typeScriptViews(src string) (code, shape string) {
	c, s := []byte(src), []byte(src)
	blank := func(b []byte, i int) {
		if b[i] != '\n' && b[i] != '\r' {
			b[i] = ' '
		}
	}
	// interpolations holds, for each ${ } of a template literal the scan is
	// inside, the bracket depth at which its closing brace returns to the
	// literal.
	var interpolations []int
	depth := 0
	inTemplate := false
	for i := 0; i < len(src); i++ {
		if inTemplate {
			switch {
			case src[i] == '\\' && i+1 < len(src):
				blank(s, i)
				blank(s, i+1)
				i++
			case src[i] == '`':
				inTemplate = false
			case src[i] == '$' && i+1 < len(src) && src[i+1] == '{':
				interpolations = append(interpolations, depth)
				depth++
				i++
				inTemplate = false
			default:
				blank(s, i)
			}
			continue
		}
		switch ch := src[i]; {
		case ch == '/' && i+1 < len(src) && src[i+1] == '/':
			for ; i < len(src) && src[i] != '\n'; i++ {
				blank(c, i)
				blank(s, i)
			}
		case ch == '/' && i+1 < len(src) && src[i+1] == '*':
			stop := len(src)
			if end := strings.Index(src[i+2:], "*/"); end >= 0 {
				stop = i + 2 + end + 2
			}
			for ; i < stop; i++ {
				blank(c, i)
				blank(s, i)
			}
			i--
		case ch == '"' || ch == '\'':
			for i++; i < len(src) && src[i] != ch && src[i] != '\n'; i++ {
				if src[i] == '\\' && i+1 < len(src) {
					blank(s, i)
					i++
				}
				blank(s, i)
			}
		case ch == '`':
			inTemplate = true
		case ch == '{' || ch == '(' || ch == '[':
			depth++
		case ch == '}' || ch == ')' || ch == ']':
			depth--
			if ch == '}' && len(interpolations) > 0 && depth == interpolations[len(interpolations)-1] {
				interpolations = interpolations[:len(interpolations)-1]
				inTemplate = true
			}
		}
	}
	return string(c), string(s)
}
