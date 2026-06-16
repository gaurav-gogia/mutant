package rustffi

import (
	"encoding/json"
	"errors"
)

const EnvelopeVersion = 1

type ProbeRequest struct {
	Version   int      `json:"version"`
	Platform  string   `json:"platform"`
	Arch      string   `json:"arch"`
	Requested []string `json:"requested"`
}

type ProbeSignal struct {
	Name       string `json:"name"`
	Detected   bool   `json:"detected"`
	Confidence int    `json:"confidence"`
	Detail     string `json:"detail"`
}

type ProbeResponse struct {
	Version int           `json:"version"`
	OK      bool          `json:"ok"`
	Error   string        `json:"error"`
	Signals []ProbeSignal `json:"signals"`
}

var ErrUnavailable = errors.New("rust anti-tamper provider unavailable")

func EncodeRequest(request ProbeRequest) (string, error) {
	if request.Version == 0 {
		request.Version = EnvelopeVersion
	}

	b, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func DecodeResponse(payload string) (ProbeResponse, error) {
	if payload == "" {
		return ProbeResponse{}, errors.New("empty rust probe response")
	}

	var response ProbeResponse
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		return ProbeResponse{}, err
	}

	if response.Version == 0 {
		response.Version = EnvelopeVersion
	}

	return response, nil
}

func RunProbe(request ProbeRequest) (ProbeResponse, error) {
	provider := newProvider()
	if provider == nil {
		return ProbeResponse{}, ErrUnavailable
	}

	requestPayload, err := EncodeRequest(request)
	if err != nil {
		return ProbeResponse{}, err
	}

	responsePayload, err := provider.Invoke(requestPayload)
	if err != nil {
		return ProbeResponse{}, err
	}

	return DecodeResponse(responsePayload)
}
