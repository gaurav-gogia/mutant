package builtin

import (
	"strings"
	"testing"
	"unicode"
)

// M26-REC-009: a withdrawal stops further grants of the record to the
// recipient, and the recipient is a person, not a byte string. Written in
// another case, with its words spaced differently, in the decomposed Unicode
// form or with an invisible character inside it, the name is the same name in
// a report, so the disclosure is refused as withdrawn -- after the respelled
// name has been given its own role, which is the only other thing that stops
// it. A name that differs in a letter is a different name and is not refused.
func TestDiscloseWithdrawnRecipientRespellingRefusedM26REC009(t *testing.T) {
	f := newDiscloseFixture(t)
	const withdrawnName = "Zoë Counsel"
	uid := keyFieldString(t, f.issue(t, "counsel", withdrawnName), "disclosure_uid")
	mustHash(t, DiscloseWithdraw(intObj(f.ledger), stringObj(uid), stringObj("classification under review")))

	combiningDiaeresis := string(rune(0x0308))
	zeroWidthSpace := string(rune(0x200B))
	noBreakSpace := string(rune(0x00A0))
	for _, variant := range []struct{ why, name string }{
		{"outer spaces", "  " + withdrawnName + " "},
		{"lower case", "zoë counsel"},
		{"upper case", "ZOË COUNSEL"},
		{"doubled space", "Zoë  Counsel"},
		{"tab between the words", "Zoë\tCounsel"},
		{"no-break space between the words", "Zoë" + noBreakSpace + "Counsel"},
		{"decomposed (NFD)", "Zoe" + combiningDiaeresis + " Counsel"},
		{"invisible format character", "Zoë Coun" + zeroWidthSpace + "sel"},
	} {
		recipient := variant.name
		f.assign(t, strings.TrimSpace(recipient))
		purposeStub(t, map[string]string{BuiltinNameDiscloseToPassphrase: testGrantPassphrase})
		_, errObj := unwrapPairNoFatal(DiscloseToPassphrase(intObj(f.ledger), f.record, stringObj("counsel"),
			stringObj(recipient)))
		if errObj == nil || !strings.Contains(errObj.Message, "was withdrawn") {
			t.Errorf("%s: the record was disclosed again to %q after its disclosure to %q was withdrawn: %v",
				variant.why, recipient, withdrawnName, errObj)
		}
	}

	// Different names are not caught by the respelling rule: a missing
	// diaeresis or a changed letter is a different name, and a different
	// word is a different person.
	f.issue(t, "counsel", "Zoe Counsel")
	f.issue(t, "counsel", "ZoeCounsel")
	f.issue(t, "counsel", "Zoë Counse1")
	f.issue(t, "counsel", "Zoë Counsel Junior")

	// The fingerprint that names the Recipient node is still taken over the
	// exact asserted spelling: the refusal compares names, it does not
	// re-key a recipient the ledger already holds.
	if disclosureRecipientFingerprint(withdrawnName) == disclosureRecipientFingerprint("zoë counsel") {
		t.Error("the recipient fingerprint is no longer taken over the exact asserted spelling")
	}
}

// Lowering can undo composition: capital J with a combining caron is already
// NFC, and its lower case composes to U+01F0. A withdrawal from the one
// spelling still refuses the other.
func TestDiscloseWithdrawnRecipientCaseAfterNFCM26REC009(t *testing.T) {
	f := newDiscloseFixture(t)
	withdrawnName := "J" + string(rune(0x030C)) + " Counsel"
	uid := keyFieldString(t, f.issue(t, "counsel", withdrawnName), "disclosure_uid")
	mustHash(t, DiscloseWithdraw(intObj(f.ledger), stringObj(uid), stringObj("classification under review")))

	for _, recipient := range []string{
		string(rune(0x01F0)) + " counsel",
		"J" + string(rune(0x030C)) + " Counsel" + string(rune(0x0001)),
		string(rune(0xFEFF)) + "J" + string(rune(0x030C)) + " Counsel",
	} {
		f.assign(t, recipient)
		purposeStub(t, map[string]string{BuiltinNameDiscloseToPassphrase: testGrantPassphrase})
		_, errObj := unwrapPairNoFatal(DiscloseToPassphrase(intObj(f.ledger), f.record, stringObj("counsel"),
			stringObj(recipient)))
		if errObj == nil || !strings.Contains(errObj.Message, "was withdrawn") {
			t.Errorf("the record was disclosed again to %q after its disclosure to %q was withdrawn: %v",
				recipient, withdrawnName, errObj)
		}
	}
}

// A name and its upper-case spelling are one name in every script, including
// those where lowering the upper case does not give the original back: Greek
// "ΝΊΚΟΣ" lowers to a medial sigma, while "Νίκος" ends in the final form. The
// upper-case respelling of a withdrawn Greek name is refused as withdrawn.
func TestDiscloseWithdrawnRecipientGreekFinalSigmaM26REC009(t *testing.T) {
	f := newDiscloseFixture(t)
	for _, withdrawnName := range []string{"Νίκος Παππάς", "Γιώργος"} {
		uid := keyFieldString(t, f.issue(t, "counsel", withdrawnName), "disclosure_uid")
		mustHash(t, DiscloseWithdraw(intObj(f.ledger), stringObj(uid), stringObj("classification under review")))

		recipient := strings.ToUpper(withdrawnName)
		f.assign(t, recipient)
		purposeStub(t, map[string]string{BuiltinNameDiscloseToPassphrase: testGrantPassphrase})
		_, errObj := unwrapPairNoFatal(DiscloseToPassphrase(intObj(f.ledger), f.record, stringObj("counsel"),
			stringObj(recipient)))
		if errObj == nil || !strings.Contains(errObj.Message, "was withdrawn") {
			t.Errorf("the record was disclosed again to %q after its disclosure to %q was withdrawn: %v",
				recipient, withdrawnName, errObj)
		}
	}
}

// The characters left out of the comparison are the ones Unicode says a
// renderer may show as nothing (Default_Ignorable_Code_Point), whatever their
// general category: a variation selector, the combining grapheme joiner, a
// Hangul filler and a soft hyphen inside the withdrawn name are still the
// withdrawn name. A format character that is drawn is not among them: U+0600
// ARABIC NUMBER SIGN and U+06DD ARABIC END OF AYAH make another name, which
// the withdrawal does not stop.
func TestDiscloseWithdrawnRecipientIgnorablesM26REC009(t *testing.T) {
	// recipientIgnorable derives Default_Ignorable_Code_Point from the
	// unicode package's tables as Unicode 15.0.0 defines it, and says so.
	if unicode.Version != "15.0.0" {
		t.Fatalf("the unicode package now implements Unicode %s: re-derive recipientIgnorable from that "+
			"version's DerivedCoreProperties.txt, correct its comment and this check", unicode.Version)
	}
	f := newDiscloseFixture(t)
	const withdrawnName = "Zoë Counsel"
	uid := keyFieldString(t, f.issue(t, "counsel", withdrawnName), "disclosure_uid")
	mustHash(t, DiscloseWithdraw(intObj(f.ledger), stringObj(uid), stringObj("classification under review")))
	for _, variant := range []struct{ why, name string }{
		{"variation selector U+FE0F", "Zoë Coun\uFE0Fsel"},
		{"combining grapheme joiner U+034F", "Zoë Coun\u034Fsel"},
		{"Hangul filler U+3164", "Zoë Coun\u3164sel"},
		{"soft hyphen U+00AD", "Zoë Coun\u00ADsel"},
	} {
		f.assign(t, variant.name)
		purposeStub(t, map[string]string{BuiltinNameDiscloseToPassphrase: testGrantPassphrase})
		_, errObj := unwrapPairNoFatal(DiscloseToPassphrase(intObj(f.ledger), f.record, stringObj("counsel"),
			stringObj(variant.name)))
		if errObj == nil || !strings.Contains(errObj.Message, "was withdrawn") {
			t.Errorf("%s: the record was disclosed again to %q after its disclosure to %q was withdrawn: %v",
				variant.why, variant.name, withdrawnName, errObj)
		}
	}

	const otherWithdrawn = "Counsel 1"
	uid = keyFieldString(t, f.issue(t, "counsel", otherWithdrawn), "disclosure_uid")
	mustHash(t, DiscloseWithdraw(intObj(f.ledger), stringObj(uid), stringObj("classification under review")))
	for _, drawn := range []string{"Counsel \u0600 1", "Counsel \u06DD 1"} {
		f.assign(t, drawn)
		purposeStub(t, map[string]string{BuiltinNameDiscloseToPassphrase: testGrantPassphrase})
		if _, errObj := unwrapPairNoFatal(DiscloseToPassphrase(intObj(f.ledger), f.record, stringObj("counsel"),
			stringObj(drawn))); errObj != nil {
			t.Errorf("%q is another name than the withdrawn %q, but its disclosure was refused: %v",
				drawn, otherWithdrawn, errObj)
		}
	}
}
