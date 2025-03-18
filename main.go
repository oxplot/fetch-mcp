package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultTimeoutSeconds = 30
	defaultMethod         = "GET"
)

func run() error {
	// Create MCP server

	mcpServer := server.NewMCPServer("Fetch", "1.0.0")

	// Add a query tool.
	mcpServer.AddTool(mcp.NewTool(
		"fetch",
		mcp.WithDescription("Fetches the content of a URL"),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description("The URL to fetch"),
		),
		mcp.WithString("headers",
			mcp.Description("JSON encoded object of headers to send"),
			mcp.DefaultString("{}"),
		),
		mcp.WithString("method",
			mcp.Description("The HTTP method to use"),
			mcp.DefaultString(defaultMethod),
		),
		mcp.WithNumber("timeout",
			mcp.Description("The timeout in seconds"),
			mcp.DefaultNumber(defaultTimeoutSeconds),
		),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

		args := request.Params.Arguments

		timeout, ok := args["timeout"].(float64)
		if !ok {
			timeout = defaultTimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()

		URL, _ := args["url"].(string)
		method, ok := args["method"].(string)
		if !ok {
			method = defaultMethod
		}
		reqHeadersStr, ok := args["headers"].(string)
		if !ok {
			reqHeadersStr = "{}"
		}
		reqHeadersMap := map[string]string{}
		if err := json.Unmarshal([]byte(reqHeadersStr), &reqHeadersMap); err != nil {
			return nil, fmt.Errorf("error parsing headers: %w", err)
		}

		// Create a new request.

		req, err := http.NewRequestWithContext(ctx, method, URL, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		for k, v := range reqHeadersMap {
			req.Header.Add(k, v)
		}

		// Fetch the URL.

		respContents := []mcp.Content{}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error fetching URL: %w", err)
		}
		defer resp.Body.Close()

		respMeta := map[string]any{}
		respMeta["headers"] = resp.Header
		respMeta["code"] = resp.StatusCode
		respMeta["status"] = resp.Status
		respMeta["http_version"] = resp.Proto
		respMetaStr, err := json.Marshal(respMeta)
		if err != nil {
			return nil, fmt.Errorf("error encoding response metadata: %w", err)
		}

		respContents = append(respContents, mcp.TextContent{
			Type: "text",
			Text: string(respMetaStr),
		})

		bb, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %w", err)
		}
		if strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
			// If image, store base64 encoded image.
			respContents = append(respContents, mcp.ImageContent{
				Type:     "image",
				Data:     base64.StdEncoding.EncodeToString(bb),
				MIMEType: resp.Header.Get("Content-Type"),
			})
		} else {
			bbStr := string(bb)
			// Try to read the response body as utf-8 text.
			if !utf8.ValidString(bbStr) {
				return nil, fmt.Errorf("response body is not valid utf-8")
			}
			respContents = append(respContents, mcp.TextContent{
				Type: "text",
				Text: bbStr,
			})
		}

		// Read the response body.

		return &mcp.CallToolResult{
			Content: respContents,
		}, nil
	})
	return server.ServeStdio(mcpServer)
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
