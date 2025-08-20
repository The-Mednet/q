package tests

import (
	"fmt"
	"testing"

	"relay/internal/variables"
	"relay/pkg/models"
)

// TestValidateNoUnresolvedVariables tests the validation function
func TestValidateNoUnresolvedVariables(t *testing.T) {
	tests := []struct {
		name        string
		message     *models.Message
		expectError bool
		expectedVar string
	}{
		{
			name: "No variables - should pass",
			message: &models.Message{
				ID:      "test-1",
				Subject: "Normal subject",
				HTML:    "<p>Normal HTML content</p>",
				Text:    "Normal text content",
			},
			expectError: false,
		},
		{
			name: "Resolved variables - should pass (no variables left)",
			message: &models.Message{
				ID:      "test-2",
				Subject: "Email with resolved content",
				HTML:    "<p>The trending question was replaced with actual content</p>",
				Text:    "The trending question was replaced with actual content",
			},
			expectError: false,
		},
		{
			name: "Unresolved variable in subject - should fail",
			message: &models.Message{
				ID:      "test-3",
				Subject: "Check out: <<TRENDING_QUESTION>>",
				HTML:    "<p>Normal content</p>",
				Text:    "Normal content",
			},
			expectError: true,
			expectedVar: "TRENDING_QUESTION",
		},
		{
			name: "Unresolved variable in HTML - should fail",
			message: &models.Message{
				ID:      "test-4",
				Subject: "Normal subject",
				HTML:    "<p>Here's a question: <<TRENDING_QUESTION>></p>",
				Text:    "Normal content",
			},
			expectError: true,
			expectedVar: "TRENDING_QUESTION",
		},
		{
			name: "Unresolved variable in Text - should fail",
			message: &models.Message{
				ID:      "test-5",
				Subject: "Normal subject",
				HTML:    "",
				Text:    "Here's a question: <<TRENDING_QUESTION>>",
			},
			expectError: true,
			expectedVar: "TRENDING_QUESTION",
		},
		{
			name: "Multiple unresolved variables - should fail",
			message: &models.Message{
				ID:      "test-6",
				Subject: "<<UNKNOWN_VAR>> and more",
				HTML:    "",
				Text:    "Here's content with <<TRENDING_QUESTION>> and <<ANOTHER_VAR>>",
			},
			expectError: true,
			expectedVar: "UNKNOWN_VAR", // Should contain at least one of them
		},
		{
			name: "Variable with parameters - should fail if unresolved",
			message: &models.Message{
				ID:      "test-7",
				Subject: "Normal subject",
				HTML:    "",
				Text:    "Here's a question: <<TRENDING_QUESTION:user:12345>>",
			},
			expectError: true,
			expectedVar: "TRENDING_QUESTION",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := variables.ValidateNoUnresolvedVariables(tt.message)

			if tt.expectError && err == nil {
				t.Errorf("Expected validation to fail but it passed")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected validation to pass but it failed with: %v", err)
			}

			if tt.expectError && err != nil {
				// Check that the expected variable is mentioned in the error
				if tt.expectedVar != "" && !containsString(err.Error(), tt.expectedVar) {
					t.Errorf("Expected error to mention variable '%s', but got: %v", tt.expectedVar, err)
				}
			}
		})
	}
}

// TestGetVariableNames tests the helper function
func TestGetVariableNames(t *testing.T) {
	tests := []struct {
		content  string
		expected []string
	}{
		{
			content:  "No variables here",
			expected: []string{},
		},
		{
			content:  "One variable: <<TRENDING_QUESTION>>",
			expected: []string{"TRENDING_QUESTION"},
		},
		{
			content:  "With params: <<TRENDING_QUESTION:user:123>>",
			expected: []string{"TRENDING_QUESTION"},
		},
		{
			content:  "Multiple: <<VAR1>> and <<VAR2>> and <<VAR1>> again",
			expected: []string{"VAR1", "VAR2"}, // Should deduplicate
		},
		{
			content:  "Invalid <<lowercase>> should not match",
			expected: []string{},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			result := variables.GetVariableNames(tt.content)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d variables, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			for _, expectedVar := range tt.expected {
				found := false
				for _, actualVar := range result {
					if actualVar == expectedVar {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected variable '%s' not found in result: %v", expectedVar, result)
				}
			}
		})
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
