package blaster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// TrendingClient handles communication with the blaster trending API
type TrendingClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// TrendingResponse represents the response from the trending API
type TrendingResponse struct {
	TopicID     int    `json:"topicId"`
	TopicName   string `json:"topicName"`
	QuestionID  int    `json:"questionId"`
	ThreadTitle string `json:"threadTitle"`
	Summary     string `json:"summary"`
	QuotedBy    string `json:"quotedBy"`
	ExpertLevel string `json:"expertLevel"`
	KeyTopics   string `json:"keyTopics"`
	DebugInfo   string `json:"debugInfo"`
}

// TrendingError represents an error response from the API
type TrendingError struct {
	Error string `json:"error"`
}

// NewTrendingClient creates a new trending API client
func NewTrendingClient(baseURL, apiKey string) *TrendingClient {
	return &TrendingClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Generous timeout for LLM generation
		},
	}
}

// GetTrendingByTopics fetches trending content for specific topic IDs
func (c *TrendingClient) GetTrendingByTopics(ctx context.Context, topicIds []int) (*TrendingResponse, error) {
	if len(topicIds) == 0 {
		return nil, fmt.Errorf("at least one topic ID is required")
	}

	// Convert topic IDs to comma-separated string
	topicIdsStr := ""
	for i, id := range topicIds {
		if i > 0 {
			topicIdsStr += ","
		}
		topicIdsStr += strconv.Itoa(id)
	}

	// Build URL with query parameters
	u, err := url.Parse(c.baseURL + "/trending/summary")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	query := u.Query()
	query.Set("topicIds", topicIdsStr)
	u.RawQuery = query.Encode()

	return c.makeRequest(ctx, u.String())
}

// GetTrendingByUser fetches trending content based on a user's specialty and subspecialty
func (c *TrendingClient) GetTrendingByUser(ctx context.Context, userID int) (*TrendingResponse, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("valid user ID is required")
	}

	// Build URL with query parameters
	u, err := url.Parse(c.baseURL + "/trending/user/summary")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	query := u.Query()
	query.Set("userId", strconv.Itoa(userID))
	u.RawQuery = query.Encode()

	return c.makeRequest(ctx, u.String())
}

// makeRequest handles the HTTP request to the trending API
func (c *TrendingClient) makeRequest(ctx context.Context, url string) (*TrendingResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add required headers
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle different HTTP status codes
	switch resp.StatusCode {
	case http.StatusOK:
		var trending TrendingResponse
		if err := json.Unmarshal(body, &trending); err != nil {
			return nil, fmt.Errorf("failed to parse trending response: %w", err)
		}
		return &trending, nil

	case http.StatusBadRequest:
		var errResp TrendingError
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("bad request (400): %s", string(body))
		}
		return nil, fmt.Errorf("bad request: %s", errResp.Error)

	case http.StatusNotFound:
		var errResp TrendingError
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("no trending topics found (404): %s", string(body))
		}
		return nil, fmt.Errorf("no trending topics found: %s", errResp.Error)

	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorized (401): check API key")

	case http.StatusInternalServerError:
		var errResp TrendingError
		if err := json.Unmarshal(body, &errResp); err != nil {
			return nil, fmt.Errorf("internal server error (500): %s", string(body))
		}
		return nil, fmt.Errorf("server error: %s", errResp.Error)

	default:
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}
}

// GetTrendingByQuestions fetches trending content for specific question IDs
// Note: This will require API updates on the server side to handle question ID parameters
func (c *TrendingClient) GetTrendingByQuestions(ctx context.Context, questionIds []int) (*TrendingResponse, error) {
	if len(questionIds) == 0 {
		return nil, fmt.Errorf("at least one question ID is required")
	}

	// Convert question IDs to comma-separated string
	questionIdsStr := ""
	for i, id := range questionIds {
		if i > 0 {
			questionIdsStr += ","
		}
		questionIdsStr += strconv.Itoa(id)
	}

	// Build URL with query parameters
	// Note: This endpoint doesn't exist yet - will need to be implemented on the API side
	u, err := url.Parse(c.baseURL + "/trending/questions/summary")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	query := u.Query()
	query.Set("questionIds", questionIdsStr)
	u.RawQuery = query.Encode()

	return c.makeRequest(ctx, u.String())
}

// GetTrendingContent is a convenience method that tries different trending approaches
// based on the provided parameters
func (c *TrendingClient) GetTrendingContent(ctx context.Context, userID *int, topicIds []int, questionIds []int) (*TrendingResponse, error) {
	// Try question-based trending first if question IDs are provided
	if len(questionIds) > 0 {
		trending, err := c.GetTrendingByQuestions(ctx, questionIds)
		if err == nil {
			return trending, nil
		}
		// Log the error but continue to fallback
		fmt.Printf("Warning: Question-based trending failed for questions %v: %v. Falling back to other methods.\n", questionIds, err)
	}

	// Try user-based trending if user ID is provided
	if userID != nil && *userID > 0 {
		trending, err := c.GetTrendingByUser(ctx, *userID)
		if err == nil {
			return trending, nil
		}
		// Log the error but continue to fallback
		fmt.Printf("Warning: User-based trending failed for user %d: %v. Falling back to topic-based.\n", *userID, err)
	}

	// Fallback to topic-based trending
	if len(topicIds) > 0 {
		return c.GetTrendingByTopics(ctx, topicIds)
	}

	return nil, fmt.Errorf("no valid user ID, topic IDs, or question IDs provided for trending content")
}