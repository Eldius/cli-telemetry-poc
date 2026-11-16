package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	searchEndpoint = "https://api.firecrawl.dev/v2/search"
)

type SearchResult struct {
	Success     bool       `json:"success"`
	Data        SearchData `json:"data"`
	CreditsUsed int        `json:"creditsUsed"`
	Id          string     `json:"id"`
}

type SearchData struct {
	Web []SearchRecord `json:"web"`
}

type SearchRecord struct {
	Url         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Position    int    `json:"position"`
}

func Search(ctx context.Context, c *http.Client, searchTerm string) (*SearchResult, error) {
	var reqBody bytes.Buffer
	if err := json.NewEncoder(&reqBody).Encode(struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}{
		Query: searchTerm,
		Limit: 5,
	}); err != nil {
		return nil, fmt.Errorf("encode search query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchEndpoint, io.NopCloser(&reqBody))
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	var result SearchResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse response body: %w", err)
	}

	return &result, nil
}
