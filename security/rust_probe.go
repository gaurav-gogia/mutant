package security

import (
	"errors"
	"os"
	"runtime"
	"strings"

	"mutant/native/rustffi"
)

const RustProbeEnableEnv = "MUTANT_ENABLE_RUST_ANTITAMPER"

type RustSignal struct {
	Name       string
	Detected   bool
	Confidence int
	Detail     string
}

func RunRustProbe(requested []string, stage string) ([]RustSignal, bool, error) {
	if strings.TrimSpace(stage) == "" {
		stage = "unknown"
	}

	if !isRustProbeEnabled() {
		return nil, false, nil
	}

	RecordRustProbeInvoked(stage)

	response, err := rustffi.RunProbe(rustffi.ProbeRequest{
		Version:   rustffi.EnvelopeVersion,
		Platform:  runtime.GOOS,
		Arch:      runtime.GOARCH,
		Requested: requested,
	})
	if err != nil {
		RecordRustProbeError(stage)
		return nil, true, err
	}

	if !response.OK {
		RecordRustProbeError(stage)
		if response.Error != "" {
			return mapRustSignals(response.Signals), true, errors.New(response.Error)
		}
		return mapRustSignals(response.Signals), true, errors.New("rust probe response not ok")
	}

	return mapRustSignals(response.Signals), true, nil
}

func isRustProbeEnabled() bool {
	return strings.TrimSpace(strings.ToLower(os.Getenv(RustProbeEnableEnv))) == "1"
}

func mapRustSignals(in []rustffi.ProbeSignal) []RustSignal {
	if len(in) == 0 {
		return nil
	}

	out := make([]RustSignal, len(in))
	for i := range in {
		out[i] = RustSignal{
			Name:       in[i].Name,
			Detected:   in[i].Detected,
			Confidence: in[i].Confidence,
			Detail:     in[i].Detail,
		}
	}
	return out
}
