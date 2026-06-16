package rustffi

import "testing"

func TestEncodeRequestSetsVersion(t *testing.T) {
	payload, err := EncodeRequest(ProbeRequest{Platform: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("EncodeRequest error: %v", err)
	}

	decoded, err := DecodeResponse(`{"version":1,"ok":true,"error":"","signals":[]}`)
	if err != nil {
		t.Fatalf("DecodeResponse error: %v", err)
	}

	if decoded.Version != EnvelopeVersion {
		t.Fatalf("wrong decoded version. got=%d, want=%d", decoded.Version, EnvelopeVersion)
	}

	if payload == "" {
		t.Fatalf("empty payload")
	}
}

func TestDecodeResponseRejectsEmpty(t *testing.T) {
	if _, err := DecodeResponse(""); err == nil {
		t.Fatalf("expected error for empty response")
	}
}

func TestRunProbeUnavailable(t *testing.T) {
	_, err := RunProbe(ProbeRequest{Version: EnvelopeVersion})
	if err == nil {
		t.Fatalf("expected unavailable provider error")
	}

	if err != ErrUnavailable {
		t.Fatalf("wrong error. got=%v, want=%v", err, ErrUnavailable)
	}
}
