package web

import (
	"context"
	"fmt"
	"gogogot/internal/tools/types"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	maxRequestBody = 1 * 1024 * 1024
)

func WebRequestTool() types.Tool {
	return types.Tool{
		Name:  "web_request",
		Label: "Making request",
		Description: "Make an HTTP request with any method (GET, POST, PUT, DELETE, PATCH) and custom headers. Returns status code, selected response headers, and body. Use for calling REST APIs, webhooks, or any HTTP endpoint.",
		Parameters: map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The full URL to request",
			},
			"method": map[string]any{
				"type":        "string",
				"description": "HTTP method: GET, POST, PUT, DELETE, PATCH (default: GET)",
			},
			"headers": map[string]any{
				"type":        "object",
				"description": `Optional HTTP headers as key-value pairs, e.g. {"Authorization": "Bearer token", "Content-Type": "application/json"}`,
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Optional request body (typically JSON). Content-Type defaults to application/json if body is provided.",
			},
		},
		Required: []string{"url"},
		Handler:  webRequest,
	}
}

func webRequest(ctx context.Context, input map[string]any) types.Result {
	rawURL, err := types.GetString(input, "url")
	if err != nil {
		return types.ErrResult(err)
	}

	method := types.GetStringOpt(input, "method")
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	body := types.GetStringOpt(input, "body")
	headers, _ := input["headers"].(map[string]any)

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return types.Result{Output: fmt.Sprintf("bad request: %v", err), IsErr: true}
	}

	req.Header.Set("User-Agent", "SofieBot/1.0")
	for k, v := range headers {
		if s, ok := v.(string); ok {
			req.Header.Set(k, s)
		}
	}

	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.Result{Output: fmt.Sprintf("http error: %v", err), IsErr: true}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRequestBody))
	if err != nil {
		return types.Result{Output: fmt.Sprintf("read body error: %v", err), IsErr: true}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "HTTP %d %s\n", resp.StatusCode, resp.Status)

	for _, name := range []string{"Content-Type", "Content-Length", "Location", "Set-Cookie"} {
		if v := resp.Header.Get(name); v != "" {
			fmt.Fprintf(&sb, "%s: %s\n", name, v)
		}
	}
	sb.WriteString("\n")

	content := string(respBody)
	if len(content) > types.MaxOutputSize-sb.Len() {
		content = content[:types.MaxOutputSize-sb.Len()] + "\n... (body truncated)"
	}
	sb.WriteString(content)

	isErr := resp.StatusCode >= 400
	return types.Result{Output: sb.String(), IsErr: isErr}
}
