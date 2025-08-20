package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"relay/internal/blaster"
	"relay/internal/variables"
	"relay/pkg/models"
)

// TestTrendingVariableReplacement tests the <<TRENDING_QUESTION>> variable replacement
func TestTrendingVariableReplacement(t *testing.T) {
	// Create a mock trending API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the API key header
		if r.Header.Get("x-api-key") != "test-api-key" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Default mock response data
		response := blaster.TrendingResponse{
			TopicID:     115,
			TopicName:   "Medical Oncology",
			QuestionID:  24650,
			ThreadTitle: "What are your top takeaways in Thoracic Cancers from ASCO 2025?",
			Summary:     "Join a timely discussion on the latest breakthroughs in thoracic cancers from ASCO 2025, including new immunotherapeutic agents and pivotal trial results. As Dr. Jarushka Naidoo notes, 'Tarlatamab in ES-SCLC [is the] first new agent approved for 2L SCLC in many years,' highlighting the rapid evolution in treatment options.",
			QuotedBy:    "Dr. Jarushka Naidoo",
			ExpertLevel: "expert",
			KeyTopics:   "thoracic cancers, ASCO 2025, immunotherapy, SCLC, clinical trials",
			DebugInfo:   "{}",
		}

		// Check if this is a question-based request
		if strings.Contains(r.URL.Path, "/trending/questions/summary") {
			// Mock response for question-based trending
			response = blaster.TrendingResponse{
				TopicID:     70,
				TopicName:   "Cardiology",
				QuestionID:  12345,
				ThreadTitle: "New Guidelines for Heart Failure Management 2025",
				Summary:     "Explore the latest heart failure management guidelines with leading cardiologists. Dr. Sarah Wilson notes, 'The new SGLT2 inhibitor protocols have shown remarkable outcomes in reducing hospitalizations,' marking a significant shift in treatment approaches.",
				QuotedBy:    "Dr. Sarah Wilson",
				ExpertLevel: "expert",
				KeyTopics:   "heart failure, SGLT2 inhibitors, cardiology guidelines",
				DebugInfo:   "{}",
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Create trending client with mock server
	trendingClient := blaster.NewTrendingClient(mockServer.URL, "test-api-key")
	variableReplacer := variables.NewVariableReplacer(trendingClient)

	// Test cases
	tests := []struct {
		name     string
		message  *models.Message
		expected string
	}{
		{
			name: "Replace trending question in HTML body",
			message: &models.Message{
				ID:      "test-1",
				Subject: "Test Email",
				HTML:    "<h1>Check out this trending question:</h1><br><<TRENDING_QUESTION>><br><p>Best regards,</p>",
				Text:    "",
				To:      []string{"doctor@example.com"},
				From:    "noreply@mednet.com",
				Metadata: map[string]interface{}{
					"recipient": map[string]interface{}{
						"user_id": 12345,
					},
				},
			},
		},
		{
			name: "Replace trending question in text body",
			message: &models.Message{
				ID:       "test-2",
				Subject:  "Test Email",
				HTML:     "",
				Text:     "Check out this trending question:\n\n<<TRENDING_QUESTION:115>>\n\nBest regards,",
				To:       []string{"doctor@example.com"},
				From:     "noreply@mednet.com",
				Metadata: map[string]interface{}{},
			},
		},
		{
			name: "Replace trending question with topic IDs in subject",
			message: &models.Message{
				ID:       "test-3",
				Subject:  "Trending: <<TRENDING_QUESTION:115,304>>",
				HTML:     "",
				Text:     "See subject for trending question.",
				To:       []string{"doctor@example.com"},
				From:     "noreply@mednet.com",
				Metadata: map[string]interface{}{},
			},
		},
		{
			name: "Replace trending question with question IDs",
			message: &models.Message{
				ID:       "test-4",
				Subject:  "Check out these questions",
				HTML:     "<p>Here are some relevant discussions:</p><br><<TRENDING_QUESTION:question:12345,67890>><br><p>Hope you find them useful!</p>",
				Text:     "",
				To:       []string{"doctor@example.com"},
				From:     "noreply@mednet.com",
				Metadata: map[string]interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			originalSubject := tt.message.Subject
			originalHTML := tt.message.HTML
			originalText := tt.message.Text

			// Apply variable replacement
			err := variableReplacer.ReplaceVariables(ctx, tt.message)
			if err != nil {
				t.Fatalf("ReplaceVariables failed: %v", err)
			}

			// Verify that content was modified if it contained variables
			if tt.message.HTML != "" && containsVariable(originalHTML) {
				if tt.message.HTML == originalHTML {
					t.Error("HTML content was not modified despite containing variables")
				}
				if !containsTrendingContent(tt.message.HTML) {
					t.Errorf("HTML does not contain expected trending content")
				}
			}

			if tt.message.Text != "" && containsVariable(originalText) {
				if tt.message.Text == originalText {
					t.Error("Text content was not modified despite containing variables")
				}
				if !containsTrendingContent(tt.message.Text) {
					t.Errorf("Text does not contain expected trending content")
				}
			}

			if containsVariable(originalSubject) {
				if tt.message.Subject == originalSubject {
					t.Error("Subject was not modified despite containing variables")
				}
				if !containsTrendingContent(tt.message.Subject) {
					t.Errorf("Subject does not contain expected trending content")
				}
			}

			t.Logf("Test passed. Subject: %s", tt.message.Subject)
			if tt.message.HTML != "" {
				t.Logf("HTML: %s", tt.message.HTML)
			}
			if tt.message.Text != "" {
				t.Logf("Text: %s", tt.message.Text)
			}
		})
	}
}

// TestVariableDetection tests the helper functions for variable detection
func TestVariableDetection(t *testing.T) {
	tests := []struct {
		content  string
		hasVars  bool
		varNames []string
	}{
		{
			content:  "Hello <<TRENDING_QUESTION>>",
			hasVars:  true,
			varNames: []string{"TRENDING_QUESTION"},
		},
		{
			content:  "No variables here",
			hasVars:  false,
			varNames: []string{},
		},
		{
			content:  "Multiple: <<TRENDING_QUESTION>> and <<TRENDING_QUESTION:115,304>>",
			hasVars:  true,
			varNames: []string{"TRENDING_QUESTION"},
		},
		{
			content:  "Question format: <<TRENDING_QUESTION:question:12345,67890>>",
			hasVars:  true,
			varNames: []string{"TRENDING_QUESTION"},
		},
		{
			content:  "User format: <<TRENDING_QUESTION:user:12345>>",
			hasVars:  true,
			varNames: []string{"TRENDING_QUESTION"},
		},
		{
			content:  "Invalid <<not_uppercase>> should not match",
			hasVars:  false,
			varNames: []string{},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			hasVars := variables.HasVariables(tt.content)
			if hasVars != tt.hasVars {
				t.Errorf("HasVariables() = %v, want %v", hasVars, tt.hasVars)
			}

			varNames := variables.GetVariableNames(tt.content)
			if len(varNames) != len(tt.varNames) {
				t.Errorf("GetVariableNames() returned %d names, want %d", len(varNames), len(tt.varNames))
			}

			for _, expectedName := range tt.varNames {
				found := false
				for _, actualName := range varNames {
					if actualName == expectedName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected variable name %s not found in %v", expectedName, varNames)
				}
			}
		})
	}
}

// Helper functions
func containsVariable(content string) bool {
	return variables.HasVariables(content)
}

func containsTrendingContent(content string) bool {
	// Check for key elements from the mock responses
	return strings.Contains(content, "thoracic cancers") ||
		strings.Contains(content, "ASCO 2025") ||
		strings.Contains(content, "Dr. Jarushka Naidoo") ||
		strings.Contains(content, "Medical Oncology") ||
		strings.Contains(content, "What are your top takeaways") ||
		// Question-based trending content
		strings.Contains(content, "heart failure") ||
		strings.Contains(content, "SGLT2 inhibitors") ||
		strings.Contains(content, "Dr. Sarah Wilson") ||
		strings.Contains(content, "Cardiology") ||
		strings.Contains(content, "Heart Failure Management")
}
