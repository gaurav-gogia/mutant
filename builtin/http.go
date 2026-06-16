package builtin

import (
	"io"
	"net/http"
	"strings"
	"time"

	"mutant/object"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func HttpGet(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	url, ok := args[0].(*object.String)
	if !ok {
		return newError("argument to `http_get` must be STRING, got %s", args[0].Type())
	}
	resp, err := httpClient.Get(url.Value)
	return httpResponseOrError(resp, err)
}

func HttpPost(args ...object.Object) object.Object {
	if len(args) != 3 {
		return newError("wrong number of arguments. got=%d, want=3", len(args))
	}
	url, ok := args[0].(*object.String)
	if !ok {
		return newError("argument 1 to `http_post` must be STRING, got %s", args[0].Type())
	}
	body, ok := args[1].(*object.String)
	if !ok {
		return newError("argument 2 to `http_post` must be STRING, got %s", args[1].Type())
	}
	contentType, ok := args[2].(*object.String)
	if !ok {
		return newError("argument 3 to `http_post` must be STRING, got %s", args[2].Type())
	}
	resp, err := httpClient.Post(url.Value, contentType.Value, strings.NewReader(body.Value))
	return httpResponseOrError(resp, err)
}

func HttpRequest(args ...object.Object) object.Object {
	if len(args) != 4 {
		return newError("wrong number of arguments. got=%d, want=4", len(args))
	}
	method, ok := args[0].(*object.String)
	if !ok {
		return newError("argument 1 to `http_request` must be STRING, got %s", args[0].Type())
	}
	url, ok := args[1].(*object.String)
	if !ok {
		return newError("argument 2 to `http_request` must be STRING, got %s", args[1].Type())
	}
	body, ok := args[2].(*object.String)
	if !ok {
		return newError("argument 3 to `http_request` must be STRING, got %s", args[2].Type())
	}
	headersArg, ok := args[3].(*object.Hash)
	if !ok {
		return newError("argument 4 to `http_request` must be HASH, got %s", args[3].Type())
	}

	req, err := http.NewRequest(strings.ToUpper(method.Value), url.Value, strings.NewReader(body.Value))
	if err != nil {
		return httpErrorResult(err)
	}

	for _, pair := range headersArg.Pairs {
		k, kOk := pair.Key.(*object.String)
		v, vOk := pair.Value.(*object.String)
		if kOk && vOk {
			req.Header.Set(k.Value, v.Value)
		}
	}

	resp, err := httpClient.Do(req)
	return httpResponseOrError(resp, err)
}

func httpResponseOrError(resp *http.Response, err error) object.Object {
	if err != nil {
		return httpErrorResult(err)
	}
	defer resp.Body.Close()

	rawBody, readErr := io.ReadAll(resp.Body)
	bodyStr := ""
	if readErr == nil {
		bodyStr = string(rawBody)
	}

	// Build headers Hash
	headerPairs := make(map[string]object.Object, len(resp.Header))
	for k, vals := range resp.Header {
		headerPairs[k] = stringObj(strings.Join(vals, ", "))
	}

	return makeHashObject(map[string]object.Object{
		"status":  intObj(int64(resp.StatusCode)),
		"body":    stringObj(bodyStr),
		"headers": makeHashObject(headerPairs),
		"error":   stringObj(""),
	})
}

func httpErrorResult(err error) object.Object {
	return makeHashObject(map[string]object.Object{
		"status":  intObj(0),
		"body":    stringObj(""),
		"headers": makeHashObject(map[string]object.Object{}),
		"error":   stringObj(err.Error()),
	})
}
