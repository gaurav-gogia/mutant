package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"mutant/global"
	"mutant/lsp/api"
)

// handleFmtCommand implements `mutant fmt [--check] [--stdout] <paths...>`.
// Without flags it rewrites each file in place with the canonical formatting.
func handleFmtCommand(args []string) int {
	set := flag.NewFlagSet("fmt", flag.ContinueOnError)
	check := set.Bool("check", false, "Do not write; exit non-zero if any file is not already formatted.")
	toStdout := set.Bool("stdout", false, "Write the formatted result to stdout instead of editing files.")
	if err := set.Parse(args[2:]); err != nil {
		return 2
	}

	paths := set.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mutant fmt [--check] [--stdout] <file-or-dir>...")
		return 2
	}

	// A problem with one argument is reported and does not stop the others
	// being formatted, and it is carried into the exit code so that a run which
	// skipped something cannot read as a clean one.
	files, problems := collectMutantSourceFiles(paths)
	exitCode := 0
	for _, problem := range problems {
		fmt.Fprintf(os.Stderr, "mutant fmt: %v\n", problem)
		exitCode = 1
	}

	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mutant fmt: %v\n", err)
			exitCode = 1
			continue
		}
		formatted, parseErrors := api.Format(string(src))
		if len(parseErrors) > 0 {
			// A file that does not parse is left exactly as it is, in every
			// mode, and the parse errors are reported in `mutant lint`'s
			// shape. `fmt` used to write the formatter's whitespace-only
			// fallback over such a file and print `formatted:` -- which,
			// having no tree to read, had quietly shortened every trailing
			// run of spaces inside its triple-quoted strings.
			for _, d := range parseErrors {
				source := ""
				if d.Source != "" {
					source = " [" + d.Source + "]"
				}
				fmt.Fprintf(os.Stderr, "%s:%d:%d: %s: %s%s\n", file, d.Line, d.Column, d.Severity, d.Message, source)
			}
			fmt.Fprintf(os.Stderr, "mutant fmt: %s does not parse; left unchanged\n", file)
			exitCode = 1
			continue
		}

		switch {
		case *toStdout:
			fmt.Print(formatted)
		case *check:
			if formatted != string(src) {
				fmt.Fprintf(os.Stderr, "would reformat: %s\n", file)
				exitCode = 1
			}
		default:
			if formatted == string(src) {
				continue
			}
			mode := fs.FileMode(0o644)
			if info, statErr := os.Stat(file); statErr == nil {
				mode = info.Mode().Perm()
			}
			if err := os.WriteFile(file, []byte(formatted), mode); err != nil {
				fmt.Fprintf(os.Stderr, "mutant fmt: %v\n", err)
				exitCode = 1
				continue
			}
			fmt.Printf("formatted: %s\n", file)
		}
	}
	return exitCode
}

// handleLintCommand implements `mutant lint [--strict] <paths...>`. It prints
// diagnostics as `file:line:col: severity: message [source]` and exits non-zero
// when any error (or, with --strict, any warning) is found.
func handleLintCommand(args []string) int {
	set := flag.NewFlagSet("lint", flag.ContinueOnError)
	strict := set.Bool("strict", false, "Treat warnings as failures (exit non-zero).")
	if err := set.Parse(args[2:]); err != nil {
		return 2
	}

	paths := set.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mutant lint [--strict] <file-or-dir>...")
		return 2
	}

	files, problems := collectMutantSourceFiles(paths)
	collectionFailed := false
	for _, problem := range problems {
		fmt.Fprintf(os.Stderr, "mutant lint: %v\n", problem)
		collectionFailed = true
	}

	// Every file at once, so a lint of one knows what the others declare. A
	// struct name, an enum name and a macro name all cross a module boundary
	// written bare, and one file at a time cannot tell one of those from a
	// typo -- it reported both.
	project := api.NewProject(files)

	failed := collectionFailed
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mutant lint: %v\n", err)
			failed = true
			continue
		}
		for _, d := range project.Lint(file, string(src)) {
			source := ""
			if d.Source != "" {
				source = " [" + d.Source + "]"
			}
			fmt.Printf("%s:%d:%d: %s: %s%s\n", file, d.Line, d.Column, d.Severity, d.Message, source)
			if d.Severity == api.SeverityError || (*strict && d.Severity == api.SeverityWarning) {
				failed = true
			}
		}
	}
	if failed {
		return 1
	}
	return 0
}

// collectMutantSourceFiles expands the given paths into a flat list of Mutant
// source files for `mutant fmt` and `mutant lint`: a `.mut` file argument is
// taken as-is, and a directory is walked for `*.mut` files.
func collectMutantSourceFiles(paths []string) ([]string, []error) {
	return collectMutantFiles(paths, global.MutantSourceCodeFileExtention)
}

// collectMutantFiles is the collection `mutant test`, `mutant fmt` and
// `mutant lint` share. walkSuffix is what a directory walk matches; a file
// named on the command line is accepted on the `.mut` extension alone, because
// the suffix is how a walk guesses and a named file needs no guessing.
//
// It returns the files it found and every problem it hit, rather than the first
// problem alone, so a caller can report all of them, work on what was found,
// and still exit non-zero. Three things it now reports used to be silent:
//
//   - a file argument that is not Mutant source. Every non-directory argument
//     was appended unchecked, so `mutant fmt notes.md` ran the Mutant
//     formatter over Markdown and wrote the result back: that file came out
//     LF-only with its trailing spaces stripped, and a trailing double space
//     is a line break in Markdown. (M26-TOOL-020)
//   - a directory the user named whose own name begins with a dot. WalkDir
//     hands the root to the callback first and the root's d.Name() is its last
//     element, so the dot rule matched the very directory that had been asked
//     for: `mutant lint .hidden` printed nothing and exited 0 while
//     `mutant lint .hidden/x.mut` reported two errors. The rule applies below
//     the root now. (M26-TOOL-019)
//   - a walk error. The callback returned nil on err, so a subdirectory that
//     could not be read contributed no files and no message. (M26-TOOL-019)
func collectMutantFiles(paths []string, walkSuffix string) ([]string, []error) {
	var files []string
	var problems []error

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if !info.IsDir() {
			if !strings.HasSuffix(p, global.MutantSourceCodeFileExtention) {
				problems = append(problems, fmt.Errorf(
					"skipping %s: not a %s file", p, global.MutantSourceCodeFileExtention))
				continue
			}
			files = append(files, p)
			continue
		}
		root := p
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				problems = append(problems, err)
				return nil
			}
			if d.IsDir() {
				// The root is never skipped by its name: it is what was asked
				// for. Below it, these are build and tool state rather than
				// source a user wrote.
				if path != root && skippedWalkDirectory(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, walkSuffix) {
				files = append(files, path)
			}
			return nil
		})
		if walkErr != nil {
			problems = append(problems, walkErr)
		}
	}
	return files, problems
}

// skippedWalkDirectory names the directories a walk does not descend into:
// package and vendor trees, and anything whose name begins with a dot, which is
// tool state rather than source.
//
// "." and ".." are not special-cased. A nested directory can never be named
// either, and the root -- which can be both, and is how `mutant test .` is
// written -- is excluded by its caller before this is asked.
func skippedWalkDirectory(name string) bool {
	return name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".")
}
