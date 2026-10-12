package builtin

import (
	"maps"
	"strconv"
	"strings"
	"testing"
)

// forgeRec025Bundle writes a role bundle definition into the attached case's
// ledger the way recordBundleLocked does -- a RoleBundle node, a BUNDLES edge
// to each view it holds, an IN_CASE edge carrying the definition -- with the
// node's and the edge's properties overridden by nodeEdit and edgeEdit. A
// field neither edit names is written as recordBundleLocked writes it.
func forgeRec025Bundle(t *testing.T, ledger *ledgerSession, roleName string, viewCanonicals []string,
	nodeEdit, edgeEdit map[string]string) {
	t.Helper()
	role, known := roleNamed(roleName)
	if !known {
		t.Fatalf("no role %q", roleName)
	}
	custodyStore.Lock()
	defer custodyStore.Unlock()
	s := custodyStore.session
	bundle := caseBundle{Role: role, Index: len(s.bundles), DefinedAt: custodyNow()}
	for _, canonical := range viewCanonicals {
		view, found := viewByCanonicalLocked(s, canonical)
		if !found {
			t.Fatalf("no view %q", canonical)
		}
		bundle.add(view)
	}

	disclosureLedgerMu.Lock()
	defer disclosureLedgerMu.Unlock()
	if err := disclosureDeclareNames(ledger); err != nil {
		t.Fatal(err)
	}
	w, err := caseBeginWrite(ledger, s.caseUID, s.ID, bundle.DefinedAt)
	if err != nil {
		t.Fatal(err)
	}
	nodeProps := map[string]string{
		"bundle.fp":          bundle.fingerprint(),
		"bundle.role":        bundle.Role.Name,
		"bundle.role_id":     strconv.FormatUint(uint64(bundle.Role.ID), 10),
		"bundle.views":       strings.Join(bundle.labels(), ", "),
		"bundle.view_fps":    strings.Join(bundle.viewFPs(), ","),
		"bundle.description": bundle.Description,
	}
	maps.Copy(nodeProps, nodeEdit)
	id, _, err := w.tx.findOrAdd(w.g, disclosureNodeRoleBundle, "bundle.fp", nodeProps)
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{}
	for _, class := range s.classes {
		labels[class.Tag] = class.Label
	}
	classNode := w.tx.classNodes(w.g, labels)
	for _, view := range bundle.Views {
		viewN, err := w.tx.viewNode(w.g, view, disclosureViewFingerprint(view), classNode)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.tx.edge(id, viewN, disclosureEdgeBundles, nil); err != nil {
			t.Fatal(err)
		}
	}
	edgeProps := s.definitionProps("role", bundle.Role.Name, bundle.Role.Name, bundle.Description, bundle.Index,
		bundle.DefinedAt)
	maps.Copy(edgeProps, edgeEdit)
	if err := w.tx.edge(id, w.caseN, disclosureEdgeInCase, edgeProps); err != nil {
		t.Fatal(err)
	}
	if err := w.tx.commit(); err != nil {
		t.Fatal(err)
	}
}

// M26-REC-025. A role bundle read back from the ledger is believed only when
// every field that names its role agrees: the definition's label and canonical
// form, the node's role name and role id, and the fingerprint over the role
// and the views it holds. A forgery that relabels the bundle as another role
// and recomputes the fingerprint, but leaves the role id it was written with,
// was read back as the other role's bundle; so was one whose definition's
// canonical form names another role.
func TestRec025ARoleBundleThatNamesTwoRolesIsRefusedAtTheAttach(t *testing.T) {
	legal, _ := roleNamed("legal")
	partner, _ := roleNamed("external_partner")
	for _, c := range []struct {
		name     string
		nodeEdit func(legalFP string) map[string]string
		edgeEdit map[string]string
		want     string // "" when the attach must read the bundle back
	}{
		{name: "nothing forged"},
		{name: "the definition's label names another role", edgeEdit: map[string]string{"def.label": "legal"},
			want: "does not match the bundle it points at"},
		{name: "the node's role names another role",
			nodeEdit: func(string) map[string]string { return map[string]string{"bundle.role": "legal"} },
			want:     "does not match the bundle it points at"},
		{name: "label, role and role id relabelled, fingerprint kept",
			edgeEdit: map[string]string{"def.label": "legal", "def.canonical": "legal"},
			nodeEdit: func(string) map[string]string {
				return map[string]string{"bundle.role": "legal",
					"bundle.role_id": strconv.FormatUint(uint64(legal.ID), 10)}
			},
			want: "fingerprint in the ledger is not the one"},
		{name: "relabelled with the fingerprint recomputed, role id kept",
			edgeEdit: map[string]string{"def.label": "legal", "def.canonical": "legal"},
			nodeEdit: func(legalFP string) map[string]string {
				return map[string]string{"bundle.role": "legal", "bundle.fp": legalFP}
			},
			want: "role id " + strconv.FormatUint(uint64(partner.ID), 10)},
		{name: "the definition's canonical form names another role",
			edgeEdit: map[string]string{"def.canonical": "legal"}, want: `canonical form "legal"`},
		{name: "the node's role id names another role",
			nodeEdit: func(string) map[string]string {
				return map[string]string{"bundle.role_id": strconv.FormatUint(uint64(legal.ID), 10)}
			},
			want: "role id " + strconv.FormatUint(uint64(legal.ID), 10)},
	} {
		t.Run(c.name, func(t *testing.T) {
			run := openLifecycleRun(t, "examiner", "case_owner", "", "")
			run.attach(t)
			mustHash(t, ClassDefine(stringObj("open")))
			mustHash(t, ClassDefine(stringObj("pii")))
			mustHash(t, ViewDefine(stringObj("counsel"), viewArray("open", "pii")))
			custodyStore.RLock()
			counsel, _ := viewByCanonicalLocked(custodyStore.session, "counsel")
			custodyStore.RUnlock()
			asLegal := caseBundle{Role: legal}
			asLegal.add(counsel)
			var nodeEdit map[string]string
			if c.nodeEdit != nil {
				nodeEdit = c.nodeEdit(asLegal.fingerprint())
			}
			forgeRec025Bundle(t, run.session(t), "external_partner", []string{"counsel"}, nodeEdit, c.edgeEdit)

			run = run.next(t, "examiner", "case_owner")
			if c.want != "" {
				mustRefuse(t, "an attach over a bundle naming two roles", CaseAttach(intObj(run.ledger)), c.want)
				return
			}
			attached := run.attach(t)
			if mustHashIntValue(t, attached, "bundles_read") != 1 {
				t.Fatalf("the attach over an honest bundle gave %s", attached.Inspect())
			}
		})
	}
}
