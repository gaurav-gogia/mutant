package builtin

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"mutant/object"
)

// detect_injection used to call almost any buffer of random bytes past a few
// hundred KiB an injection. Every bare "MZ" byte pair counted toward "multiple
// PE headers" (+40) and a single cld; call pair toward the shellcode score
// (+20), and random bytes hold each about once per 64 KiB, so 1 MiB and 4 MiB
// of seeded random bytes both came back detected=true, score=60, with a dozen
// or more "PE headers" of which mem_find_pe confirmed none (M26-ART-026).
func TestDetectInjectionSeededRandomBytesAreNotAnInjection(t *testing.T) {
	for _, size := range []int{1 << 20, 4 << 20} {
		t.Run(fmt.Sprintf("%dMiB", size>>20), func(t *testing.T) {
			path := injChanceWrite(t, injChanceRandomBytes(size, 26))
			h := injChanceDetect(t, path)

			confirmed := injChanceConfirmedByMemFindPE(t, path)
			if got := dtMustHashInt(t, h, "pe_headers"); got != confirmed {
				t.Errorf("pe_headers = %d, but mem_find_pe confirms %d: an MZ marker is not a PE "+
					"header until its e_lfanew leads to PE\\0\\0", got, confirmed)
			}
			if dtMustHashBool(t, h, "detected") {
				t.Fatalf("%d bytes of seeded random data reported as an injection: %s", size, h.Inspect())
			}
			if reasons := dtMustHashValue(t, h, "reasons").(*object.Array); dtReasonsContain(reasons, "multiple_pe_headers") {
				t.Errorf("reasons = %s on random bytes holding %d confirmed PE headers", reasons.Inspect(), confirmed)
			}

			// Random bytes this long hold cld; call pairs, about one per 64 KiB.
			// They are chance, so they are reported as chance and do not score.
			if injChanceNames(t, h, "matched_signatures")["classic_prologue"] {
				t.Errorf("classic_prologue scored at the density random bytes hold: %s", h.Inspect())
			}
			if !injChanceNames(t, h, "chance_signatures")["classic_prologue"] {
				t.Errorf("the cld; call pairs in random bytes are missing from chance_signatures: %s", h.Inspect())
			}
		})
	}
}

// Only an MZ marker whose e_lfanew leads to "PE\0\0" is a PE header, and two
// markers that lead to the same header are one header.
func TestDetectInjectionCountsOnlyConfirmedPEHeaders(t *testing.T) {
	t.Run("bare MZ pairs", func(t *testing.T) {
		data := make([]byte, 16<<10)
		for off := 0; off+2 <= len(data); off += 256 {
			data[off], data[off+1] = 'M', 'Z'
		}
		h := injChanceDetect(t, injChanceWrite(t, data))
		if got := dtMustHashInt(t, h, "pe_headers"); got != 0 {
			t.Fatalf("pe_headers = %d for 64 MZ markers none of which leads to PE\\0\\0, want 0", got)
		}
		if dtReasonsContain(dtMustHashValue(t, h, "reasons").(*object.Array), "multiple_pe_headers") {
			t.Fatalf("multiple_pe_headers given for a buffer with no PE header: %s", h.Inspect())
		}
	})

	t.Run("two PE images", func(t *testing.T) {
		data := append(dtPEImage(), make([]byte, 512)...)
		data = append(data, dtPEImage()...)
		path := injChanceWrite(t, data)
		h := injChanceDetect(t, path)
		if got := dtMustHashInt(t, h, "pe_headers"); got != 2 {
			t.Fatalf("pe_headers = %d, want 2", got)
		}
		if confirmed := injChanceConfirmedByMemFindPE(t, path); confirmed != 2 {
			t.Fatalf("mem_find_pe confirms %d, want 2: the fixture is not what it says", confirmed)
		}
		if !dtReasonsContain(dtMustHashValue(t, h, "reasons").(*object.Array), "multiple_pe_headers") {
			t.Fatalf("no multiple_pe_headers for two confirmed PE headers: %s", h.Inspect())
		}
		if got := dtMustHashInt(t, h, "score"); got != 40 {
			t.Fatalf("score = %d, want 40 for the PE headers alone", got)
		}
	})

	t.Run("two markers, one header", func(t *testing.T) {
		data := make([]byte, 0x86)
		data[0], data[1] = 'M', 'Z'
		binary.LittleEndian.PutUint32(data[0x3C:], 0x80)
		data[0x10], data[0x11] = 'M', 'Z'
		binary.LittleEndian.PutUint32(data[0x10+0x3C:], 0x70)
		copy(data[0x80:], []byte{'P', 'E', 0, 0, 0x4c, 0x01})
		h := injChanceDetect(t, injChanceWrite(t, data))
		if got := dtMustHashInt(t, h, "pe_headers"); got != 1 {
			t.Fatalf("pe_headers = %d for two MZ markers that both lead to the PE header at 0x80, want 1", got)
		}
		if dtReasonsContain(dtMustHashValue(t, h, "reasons").(*object.Array), "multiple_pe_headers") {
			t.Fatalf("multiple_pe_headers given for one PE header: %s", h.Inspect())
		}
	})
}

// The two-byte cld; call signature counts by density, not by presence: in
// 1 MiB, random bytes hold 16 of them on average, and 31 is the fewest that
// random bytes reach in no more than one buffer in a thousand.
func TestDetectInjectionTwoBytePrologueScoresByDensity(t *testing.T) {
	prologue := []byte{0xfc, 0xe8}
	spread := func(hits int) []byte {
		data := make([]byte, 1<<20)
		step := len(data) / hits
		for i := 0; i < hits; i++ {
			copy(data[i*step:], prologue)
		}
		return data
	}

	t.Run("as many as chance", func(t *testing.T) {
		h := injChanceDetect(t, injChanceWrite(t, spread(16)))
		if injChanceNames(t, h, "matched_signatures")["classic_prologue"] {
			t.Fatalf("16 cld; call pairs in 1 MiB scored, but random bytes hold that many: %s", h.Inspect())
		}
		if got := dtMustHashInt(t, h, "score"); got != 0 {
			t.Fatalf("score = %d, want 0: %s", got, h.Inspect())
		}
		if got := dtMustHashInt(t, h, "shellcode_hits"); got != 0 {
			t.Fatalf("shellcode_hits = %d, want 0: the hits chance explains are not shellcode hits", got)
		}
		entry := injChanceEntry(t, h, "chance_signatures", "classic_prologue")
		if got := dtMustHashInt(t, entry, "count"); got != 16 {
			t.Fatalf("chance_signatures count = %d, want 16", got)
		}
		// (1 MiB - 1) positions, each a match one time in 65536.
		if got, want := dtMustHashFloat(t, entry, "expected"), float64(1<<20-1)/65536; got != want {
			t.Fatalf("chance_signatures expected = %v, want %v", got, want)
		}
	})

	t.Run("the fewest that count", func(t *testing.T) {
		for _, tc := range []struct {
			hits   int
			counts bool
		}{{30, false}, {31, true}} {
			h := injChanceDetect(t, injChanceWrite(t, spread(tc.hits)))
			if got := injChanceNames(t, h, "matched_signatures")["classic_prologue"]; got != tc.counts {
				t.Errorf("%d cld; call pairs in 1 MiB: scored = %v, want %v: %s", tc.hits, got, tc.counts, h.Inspect())
			}
			if got := injChanceNames(t, h, "chance_signatures")["classic_prologue"]; got == tc.counts {
				t.Errorf("%d cld; call pairs in 1 MiB: listed as chance = %v, want %v: %s", tc.hits, got, !tc.counts, h.Inspect())
			}
		}
	})

	t.Run("far more than chance", func(t *testing.T) {
		h := injChanceDetect(t, injChanceWrite(t, spread(4096)))
		entry := injChanceEntry(t, h, "matched_signatures", "classic_prologue")
		if got := dtMustHashInt(t, entry, "count"); got != 4096 {
			t.Fatalf("classic_prologue count = %d, want 4096", got)
		}
		if got := dtMustHashInt(t, h, "score"); got != 20 {
			t.Fatalf("score = %d, want 20", got)
		}
		if injChanceNames(t, h, "chance_signatures")["classic_prologue"] {
			t.Fatalf("classic_prologue listed as chance as well as scored: %s", h.Inspect())
		}
	})
}

// A single cld; call counts only where random bytes would hold one in no more
// than one buffer in a thousand, which is a buffer of at most 66 bytes. In
// 4 KiB, random bytes hold one in about 6 buffers in 100, so a lone cld; call
// there is chance: it is listed with the count random bytes average, and it
// scores nothing.
func TestDetectInjectionOneCldCallCountsOnlyInATinyBuffer(t *testing.T) {
	for _, tc := range []struct {
		size   int
		counts bool
	}{{66, true}, {67, false}, {4 << 10, false}} {
		t.Run(fmt.Sprintf("%d bytes", tc.size), func(t *testing.T) {
			data := make([]byte, tc.size)
			copy(data[tc.size/2:], []byte{0xfc, 0xe8})
			h := injChanceDetect(t, injChanceWrite(t, data))
			if got := injChanceNames(t, h, "matched_signatures")["classic_prologue"]; got != tc.counts {
				t.Fatalf("one cld; call in %d bytes: scored = %v, want %v: %s", tc.size, got, tc.counts, h.Inspect())
			}
			wantScore := int64(0)
			if tc.counts {
				wantScore = 20
			}
			if got := dtMustHashInt(t, h, "score"); got != wantScore {
				t.Fatalf("score = %d, want %d: %s", got, wantScore, h.Inspect())
			}
			if tc.counts {
				return
			}
			entry := injChanceEntry(t, h, "chance_signatures", "classic_prologue")
			if got, want := dtMustHashFloat(t, entry, "expected"), float64(tc.size-1)/65536; got != want {
				t.Fatalf("chance_signatures expected = %v, want %v", got, want)
			}
		})
	}
}

// Only a signature shorter than four bytes is weighed against chance. A
// four-byte or longer one counts on presence however long the buffer is, since
// random bytes hold a given four-byte pattern about once per 4 GiB. So one
// fnstenv GetPC beside two confirmed PE images is detected (40 + 30) in 6 MiB
// as it is in 2 MiB, and one xor/push in 6 MiB still scores its 15.
func TestDetectInjectionLongSignaturesCountOnPresence(t *testing.T) {
	// A shikata_ga_nai decoder head: fcmovb st(0), st(3), then fnstenv [esp-0Ch].
	shikata := []byte{0xda, 0xc3, 0xd9, 0x74, 0x24, 0xf4, 0x5d, 0x29, 0xc9, 0xb1, 0x52}
	for _, size := range []int{2 << 20, 6 << 20} {
		t.Run(fmt.Sprintf("fnstenv beside two PE images, %d MiB", size>>20), func(t *testing.T) {
			data := make([]byte, size)
			copy(data, dtPEImage())
			copy(data[64<<10:], dtPEImage())
			copy(data[size/2:], shikata)
			path := injChanceWrite(t, data)
			if confirmed := injChanceConfirmedByMemFindPE(t, path); confirmed != 2 {
				t.Fatalf("mem_find_pe confirms %d, want 2: the fixture is not what it says", confirmed)
			}
			h := injChanceDetect(t, path)
			if !dtMustHashBool(t, h, "detected") {
				t.Fatalf("one fnstenv GetPC beside two confirmed PE images in %d MiB not detected: %s", size>>20, h.Inspect())
			}
			if got := dtMustHashInt(t, h, "score"); got != 70 {
				t.Fatalf("score = %d, want 70: %s", got, h.Inspect())
			}
			if got := dtMustHashInt(t, injChanceEntry(t, h, "matched_signatures", "getpc_fnstenv"), "count"); got != 1 {
				t.Fatalf("getpc_fnstenv count = %d, want 1", got)
			}
			if names := injChanceNames(t, h, "chance_signatures"); len(names) != 0 {
				t.Fatalf("chance_signatures = %v, want none: a four-byte signature is not weighed against chance", names)
			}
		})
	}

	t.Run("xor/push alone, 6 MiB", func(t *testing.T) {
		data := make([]byte, 6<<20)
		copy(data[3<<20:], []byte{0x31, 0xc0, 0x50, 0x68})
		h := injChanceDetect(t, injChanceWrite(t, data))
		if got := dtMustHashInt(t, injChanceEntry(t, h, "matched_signatures", "xor_push"), "count"); got != 1 {
			t.Fatalf("xor_push count = %d, want 1", got)
		}
		if got := dtMustHashInt(t, h, "score"); got != 15 {
			t.Fatalf("score = %d, want 15: %s", got, h.Inspect())
		}
	})
}

// Every key detect_injection returns is one its contract declares, so the
// editor's hover teaches chance_signatures along with the rest.
func TestDetectInjectionDeclaresEveryFieldItReturns(t *testing.T) {
	spec, ok := ReturnSpec(BuiltinNameDetectInjection)
	if !ok {
		t.Fatal("detect_injection has no return contract")
	}
	declared := map[string]bool{}
	for _, field := range spec.Fields {
		declared[field] = true
	}
	h := injChanceDetect(t, injChanceWrite(t, []byte{0xfc, 0xe8, 0x00}))
	for _, pair := range h.Pairs {
		if key := pair.Key.(*object.String).Value; !declared[key] {
			t.Errorf("detect_injection returns %q, which its contract (%v) does not declare", key, spec.Fields)
		}
	}
}

func injChanceRandomBytes(size int, seed uint64) []byte {
	r := rand.New(rand.NewPCG(seed, 0x6d7574616e74))
	data := make([]byte, size)
	var word [8]byte
	for off := 0; off < size; off += len(word) {
		binary.LittleEndian.PutUint64(word[:], r.Uint64())
		copy(data[off:], word[:])
	}
	return data
}

func injChanceWrite(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func injChanceDetect(t *testing.T, path string) *object.Hash {
	t.Helper()
	payload, errObj := unwrapPair(t, DetectInjection(makeHashObject(map[string]object.Object{
		"mem_path": stringObj(path),
	})))
	if errObj != nil {
		t.Fatalf("detect_injection error: %s", errObj.Inspect())
	}
	return dtMustHash(t, payload)
}

func injChanceConfirmedByMemFindPE(t *testing.T, path string) int64 {
	t.Helper()
	payload, errObj := unwrapPair(t, MemFindPE(stringObj(path)))
	if errObj != nil {
		t.Fatalf("mem_find_pe error: %s", errObj.Inspect())
	}
	return dtMustHashInt(t, dtMustHash(t, payload), "confirmed")
}

// injChanceNames is the set of signature names listed under key.
func injChanceNames(t *testing.T, h *object.Hash, key string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	value, present := detectHashValueByKey(h, key)
	if !present {
		return names
	}
	for _, entry := range value.(*object.Array).Elements {
		names[dtMustHashString(t, dtMustHash(t, entry), "name")] = true
	}
	return names
}

// injChanceEntry is the entry for the signature name listed under key.
func injChanceEntry(t *testing.T, h *object.Hash, key, name string) *object.Hash {
	t.Helper()
	list, ok := dtMustHashValue(t, h, key).(*object.Array)
	if !ok {
		t.Fatalf("%s is not an ARRAY: %s", key, h.Inspect())
	}
	for _, entry := range list.Elements {
		if e := dtMustHash(t, entry); dtMustHashString(t, e, "name") == name {
			return e
		}
	}
	t.Fatalf("%s has no %s entry: %s", key, name, h.Inspect())
	return nil
}
