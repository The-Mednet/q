package variables

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"smtp_relay/internal/blaster"
	"smtp_relay/pkg/models"
)

// VariableReplacer handles replacement of template variables in email content
type VariableReplacer struct {
	trendingClient *blaster.TrendingClient
}

// NewVariableReplacer creates a new variable replacer
func NewVariableReplacer(trendingClient *blaster.TrendingClient) *VariableReplacer {
	return &VariableReplacer{
		trendingClient: trendingClient,
	}
}

// ReplaceVariables processes email content and replaces template variables
func (vr *VariableReplacer) ReplaceVariables(ctx context.Context, msg *models.Message) error {
	log.Printf("DEBUG: ReplaceVariables called for message %s with UserID='%s'", msg.ID, msg.UserID)
	
	if vr.trendingClient == nil {
		log.Printf("Warning: Trending client not configured, skipping variable replacement for message %s", msg.ID)
		return nil
	}

	// Process subject
	if msg.Subject != "" {
		processed, err := vr.processContent(ctx, msg.Subject, msg)
		if err != nil {
			log.Printf("Warning: Failed to process variables in subject for message %s: %v", msg.ID, err)
			// Continue with original subject if replacement fails
		} else {
			msg.Subject = processed
		}
	}

	// Process HTML body
	if msg.HTML != "" {
		processed, err := vr.processContent(ctx, msg.HTML, msg)
		if err != nil {
			log.Printf("Warning: Failed to process variables in HTML body for message %s: %v", msg.ID, err)
			// Continue with original HTML if replacement fails
		} else {
			msg.HTML = processed
		}
	}

	// Process text body
	if msg.Text != "" {
		processed, err := vr.processContent(ctx, msg.Text, msg)
		if err != nil {
			log.Printf("Warning: Failed to process variables in text body for message %s: %v", msg.ID, err)
			// Continue with original text if replacement fails
		} else {
			msg.Text = processed
		}
	}

	return nil
}

// processContent handles the actual variable replacement in content
func (vr *VariableReplacer) processContent(ctx context.Context, content string, msg *models.Message) (string, error) {
	// Pattern to match <<VARIABLE_NAME>> or <<VARIABLE_NAME:param1,param2>>
	pattern := regexp.MustCompile(`<<([A-Z_]+)(?::([^>]+))?>>`)
	
	result := content
	matches := pattern.FindAllStringSubmatch(content, -1)
	
	log.Printf("DEBUG: Found %d variable matches in content", len(matches))
	
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		
		fullMatch := match[0]    // e.g., "<<TRENDING_QUESTION>>"
		varName := match[1]      // e.g., "TRENDING_QUESTION"
		params := ""
		if len(match) > 2 {
			params = match[2]     // e.g., "15,23,45" for topic IDs
		}
		
		replacement, err := vr.getVariableReplacement(ctx, varName, params, msg)
		if err != nil {
			log.Printf("Warning: Failed to replace variable %s: %v", varName, err)
			// Keep the original variable if replacement fails
			continue
		}
		
		result = strings.ReplaceAll(result, fullMatch, replacement)
		log.Printf("Replaced variable %s in message %s", varName, msg.ID)
	}
	
	return result, nil
}

// getVariableReplacement returns the replacement content for a specific variable
func (vr *VariableReplacer) getVariableReplacement(ctx context.Context, varName, params string, msg *models.Message) (string, error) {
	switch varName {
	case "TRENDING_QUESTION":
		return vr.getTrendingQuestionReplacement(ctx, params, msg)
	default:
		return "", fmt.Errorf("unknown variable: %s", varName)
	}
}

// getTrendingQuestionReplacement handles the <<TRENDING_QUESTION>> variable
func (vr *VariableReplacer) getTrendingQuestionReplacement(ctx context.Context, params string, msg *models.Message) (string, error) {
	// Parse parameters - could be topic IDs, user ID, or question IDs
	var userID *int
	var topicIds []int
	var questionIds []int
	
	if params != "" {
		// Check if params start with "user:" for user-based trending
		if strings.HasPrefix(params, "user:") {
			userIDStr := strings.TrimPrefix(params, "user:")
			if id, err := strconv.Atoi(userIDStr); err == nil && id > 0 {
				userID = &id
			} else {
				return "", fmt.Errorf("invalid user ID in parameters: %s", userIDStr)
			}
		} else if strings.HasPrefix(params, "question:") {
			// Parse as comma-separated question IDs
			questionIDsStr := strings.TrimPrefix(params, "question:")
			parts := strings.Split(questionIDsStr, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if id, err := strconv.Atoi(part); err == nil && id > 0 {
					questionIds = append(questionIds, id)
				} else {
					log.Printf("Warning: Invalid question ID '%s' in parameters, skipping", part)
				}
			}
		} else {
			// Parse as comma-separated topic IDs (backward compatibility)
			parts := strings.Split(params, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if id, err := strconv.Atoi(part); err == nil && id > 0 {
					topicIds = append(topicIds, id)
				} else {
					log.Printf("Warning: Invalid topic ID '%s' in parameters, skipping", part)
				}
			}
		}
	}
	
	// Try to extract user ID from message if not provided in params
	if userID == nil {
		// First try the UserID field from the message
		if msg.UserID != "" {
			if id, err := strconv.Atoi(msg.UserID); err == nil && id > 0 {
				userID = &id
			}
		}
		
		// Fallback to metadata if UserID field is empty
		if userID == nil {
			if recipientData, ok := msg.Metadata["recipient"]; ok {
				if recipientMap, ok := recipientData.(map[string]interface{}); ok {
					if userIDValue, exists := recipientMap["user_id"]; exists {
						switch v := userIDValue.(type) {
						case int:
							userID = &v
						case float64:
							id := int(v)
							userID = &id
						case string:
							if id, err := strconv.Atoi(v); err == nil {
								userID = &id
							}
						}
					}
				}
			}
		}
	}
	
	// Get trending content from the API
	trending, err := vr.trendingClient.GetTrendingContent(ctx, userID, topicIds, questionIds)
	if err != nil {
		return "", fmt.Errorf("failed to get trending content: %w", err)
	}
	
	// Format the trending content for email
	return vr.formatTrendingContent(trending), nil
}

// formatTrendingContent formats the trending response for email inclusion
func (vr *VariableReplacer) formatTrendingContent(trending *blaster.TrendingResponse) string {
	// Create a clean, professional summary with the trending question
	var builder strings.Builder
	
	// Add topic prefix if available
	if trending.TopicName != "" {
		builder.WriteString(fmt.Sprintf("A recent question from %s:\n\n", trending.TopicName))
	}
	
	// Add the thread title as a header (no emoji, no markdown)
	builder.WriteString(fmt.Sprintf("%s\n\n", trending.ThreadTitle))
	
	// Add the summary (just the content, no extra formatting)
	builder.WriteString(trending.Summary)
	
	return builder.String()
}

// HasVariables checks if content contains any replaceable variables
func HasVariables(content string) bool {
	pattern := regexp.MustCompile(`<<[A-Z_]+(?::[^>]+)?>>`)
	return pattern.MatchString(content)
}

// GetVariableNames extracts all variable names from content
func GetVariableNames(content string) []string {
	pattern := regexp.MustCompile(`<<([A-Z_]+)(?::[^>]+)?>>`)
	matches := pattern.FindAllStringSubmatch(content, -1)
	
	var names []string
	seen := make(map[string]bool)
	
	for _, match := range matches {
		if len(match) >= 2 {
			name := match[1]
			if !seen[name] {
				names = append(names, name)
				seen[name] = true
			}
		}
	}
	
	return names
}