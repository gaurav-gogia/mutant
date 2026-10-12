package analyzer

// evidenceMutation reports a write, delete or move aimed at a path the same
// program opened as evidence.
//
// This is the rule only this language can write. In a general-purpose language
// a path is a path; here `ewf_open`, `raw_open`, `ntfs_open` and their
// relatives say out loud that the file on the other end is an exhibit, and
// writing to an exhibit is not a bug in the ordinary sense -- the program does
// exactly what it was told, and what it destroys is the thing the analysis was
// about. A hash taken afterwards no longer matches the one in the notes, and no
// amount of care later puts that back.
//
// Certainty comes from comparing what the author wrote rather than what it will
// evaluate to: the same identifier, or the same string literal, on both calls.
// A path *derived* from the evidence path, or a directory containing it, is a
// guess about two run-time strings and is not reported.
//
// Identifiers are matched within a scope and literals across the file. A
// literal is the same file wherever it appears, but a parameter called `path`
// in one function has nothing to do with a parameter called `path` in another,
// and matching those would invent a finding out of a common name.

import (
	"fmt"

	mast "mutant/ast"
	"mutant/builtin"

	lsp "github.com/tliron/glsp/protocol_3_16"
)

// evidenceOpeners are the builtins whose first argument is a path to something
// being examined as evidence.
//
// Curated rather than taken wholesale from resourceFamilies, which also holds
// families that open things the analyst owns: db_open_disk is the case notes,
// cache_open is a working cache, chan_new and net_connect are not files at all.
// Writing to any of those is ordinary work.
//
// Not every entry hands back a handle. vhdi_probe reads an image's headers and
// returns a description, so nothing else in the analyzer has reason to know it
// touched a file -- and the file it touched is an exhibit exactly as much as
// one that was opened.
var evidenceOpeners = map[string]string{
	builtin.BuiltinNameRawOpen: "a raw disk image",
	builtin.BuiltinNameEwfOpen: "an EWF/E01 image",
	// A set damaged enough to need ewf_open_partial is the last thing that
	// should be written over: there is less of it left to re-read.
	builtin.BuiltinNameEwfOpenPartial: "an incomplete EWF/E01 image",
	builtin.BuiltinNameVhdiOpen:       "a VHD/VHDX image",
	builtin.BuiltinNameVhdiProbe:      "a VHD/VHDX image",
	builtin.BuiltinNameTableOpen:      "a partition table",
	builtin.BuiltinNameNtfsOpen:       "an NTFS filesystem",
	builtin.BuiltinNameFatOpen:        "a FAT filesystem",
	builtin.BuiltinNameXfatOpen:       "an exFAT filesystem",
	builtin.BuiltinNameExtOpen:        "an ext filesystem",
	builtin.BuiltinNameHfsOpen:        "an HFS+ filesystem",
	builtin.BuiltinNameXfsOpen:        "an XFS filesystem",
	builtin.BuiltinNameHiveOpen:       "a registry hive",
	builtin.BuiltinNameZipOpen:        "a zip archive",
	builtin.BuiltinNameTarOpen:        "a tar archive",
}

// evidenceMutator is a builtin that changes a file, and which of its arguments
// name the file it changes.
type evidenceMutator struct {
	paths []int
	verb  string
}

var evidenceMutators = map[string]evidenceMutator{
	builtin.BuiltinNameFsWrite:  {paths: []int{0}, verb: "overwrites"},
	builtin.BuiltinNameFsAppend: {paths: []int{0}, verb: "appends to"},
	builtin.BuiltinNameFsDelete: {paths: []int{0}, verb: "deletes"},
	// fs_move(src, dst): moving the evidence away is as destructive as
	// deleting it, and moving something *onto* it replaces it.
	builtin.BuiltinNameFsMove: {paths: []int{0, 1}, verb: "moves"},
	// fs_copy(src, dst): only the destination is written. Copying evidence
	// somewhere else is the correct thing to do and must stay quiet.
	builtin.BuiltinNameFsCopy: {paths: []int{1}, verb: "copies over"},
}

// pathKey canonicalises a path expression to something two call sites can be
// compared on. Anything else -- a call, an index, a concatenation, an
// interpolation -- has no key, and a mutation with no key is never reported.
//
// A name is keyed by the DECLARATION it refers to and not by its spelling. The
// file header above claims certainty -- "Certainty comes from comparing what the
// author wrote" -- and a spelling is not that:
//
//	for (path in images)  { raw_open(path); }
//	for (path in outputs) { fs_write(path, "report"); }
//
// is two variables, written by an author who meant two, and keying on the word
// `path` made the second loop a write to the evidence the first loop opened.
// Iterate the images, then write the reports is the shape every triage script
// has. A rebinding `let` does the same thing with one name.
//
// A name the graph cannot resolve has no key, which is the quiet direction this
// rule errs in everywhere else.
func pathKey(snapshot *Snapshot, expr mast.Expression) (string, bool) {
	switch node := expr.(type) {
	case *mast.Identifier:
		if node == nil || node.Value == "" || node.Value == "_" {
			return "", false
		}
		return declarationKeyOf(snapshot, node)
	case *mast.StringLiteral:
		if node == nil || node.Value == "" {
			return "", false
		}
		return "path:" + node.Value, true
	}
	return "", false
}

// declarationKeyOf keys an identifier on the declaration it binds to.
//
// DeclID already distinguishes two declarations of one name in one scope by its
// Seq, which is the whole of what this rule was missing, so the key is that id
// written out. The graph is built once per document and the undefined-name rule
// in this same diagnostics pass already asks for it, so this costs nothing it
// was not already paying.
func declarationKeyOf(snapshot *Snapshot, ident *mast.Identifier) (string, bool) {
	if snapshot == nil || snapshot.Program == nil {
		return "", false
	}
	rng, ok := snapshot.Program.RangeOf(ident)
	if !ok {
		return "", false
	}
	declared, resolved := snapshot.Graph().Resolve(rng.Start.Line, rng.Start.Column)
	if !resolved || declared == nil || declared.ID.IsZero() {
		return "", false
	}
	id := declared.ID
	return fmt.Sprintf("decl:%s|%s|%s|%d", id.Module, id.Scope, id.Name, id.Seq), true
}

// openedEvidence is one opener call and the path it was given.
type openedEvidence struct {
	opener  string
	subject string
}

func lintEvidenceMutation(snapshot *Snapshot, lintConfig LintConfig) []lsp.Diagnostic {
	if snapshot == nil || snapshot.Program == nil || snapshot.Program.NodePositions == nil {
		return nil
	}

	severity, ok := lintConfig.severityForRule("evidenceMutation")
	if !ok {
		return nil
	}

	statements := snapshot.Program.Statements
	shadowed := namesBoundAnywhere(statements)

	// Literal paths first, over the whole document: the same string names the
	// same file wherever the two calls happen to sit.
	fileWide := make(map[string]openedEvidence)
	forEachBuiltinCall(statements, shadowed,
		func(name string, _ mast.Node, call *mast.CallExpression, _ map[string]mast.Expression) {
			subject, isOpener := evidenceOpeners[name]
			if !isOpener {
				return
			}
			argument := argumentAt(call, 0)
			if argument == nil {
				return
			}
			if _, literal := literalString(argument); !literal {
				return
			}
			key, ok := pathKey(snapshot, argument)
			if !ok {
				return
			}
			fileWide[key] = openedEvidence{opener: name, subject: subject}
		})

	source := "mutant-lint"
	var result []lsp.Diagnostic

	forEachScope(statements, func(scope []mast.Statement) {
		// Then names, within this scope only.
		inScope := make(map[string]openedEvidence, len(fileWide))
		for key, evidence := range fileWide {
			inScope[key] = evidence
		}

		isShadowed := func(candidate string) bool {
			_, taken := shadowed[candidate]
			return taken
		}

		forEachTopStatement(scope, func(stmt mast.Statement) {
			walkExpressions(stmt, false, func(expr mast.Expression) {
				call, ok := expr.(*mast.CallExpression)
				if !ok || call == nil {
					return
				}
				name, _, ok := builtinCallee(call.Function, isShadowed)
				if !ok {
					return
				}
				subject, isOpener := evidenceOpeners[name]
				if !isOpener {
					return
				}
				argument := argumentAt(call, 0)
				if argument == nil {
					return
				}
				if key, ok := pathKey(snapshot, argument); ok {
					inScope[key] = openedEvidence{opener: name, subject: subject}
				}
			})
		})

		if len(inScope) == 0 {
			return
		}

		forEachTopStatement(scope, func(stmt mast.Statement) {
			walkExpressions(stmt, false, func(expr mast.Expression) {
				call, ok := expr.(*mast.CallExpression)
				if !ok || call == nil {
					return
				}
				name, anchor, ok := builtinCallee(call.Function, isShadowed)
				if !ok {
					return
				}
				mutator, isMutator := evidenceMutators[name]
				if !isMutator {
					return
				}

				for _, index := range mutator.paths {
					argument := argumentAt(call, index)
					if argument == nil {
						continue
					}
					key, ok := pathKey(snapshot, argument)
					if !ok {
						continue
					}
					evidence, opened := inScope[key]
					if !opened {
						continue
					}

					rng, ok := snapshot.Program.RangeOf(anchor)
					if !ok {
						continue
					}
					result = append(result, lsp.Diagnostic{
						Range:    snapshot.Range(rng),
						Severity: severity,
						Source:   &source,
						Message: fmt.Sprintf(
							"`%s` %s the same path `%s` opened as %s. Evidence is written once and read forever: a hash taken after this no longer matches the one in the notes, and nothing later restores what was there. Work on a copy -- `fs_copy` the image first and open the copy -- or write the output somewhere of its own.",
							name, mutator.verb, evidence.opener, evidence.subject),
					})
					// One report per call: the second path of an fs_move over
					// the same evidence is the same mistake.
					return
				}
			})
		})
	})

	return result
}
