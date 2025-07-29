package llm

import (
	"context"
	"fmt"
	"log"
	"strings"

	"smtp_relay/internal/config"
	"smtp_relay/pkg/models"

	blasterLLM "blaster/pkg/llm"
	"blaster/pkg/types"
)

type Personalizer struct {
	config  *config.LLMConfig
	enabled bool
}

func NewPersonalizer(cfg *config.LLMConfig) *Personalizer {
	return &Personalizer{
		config:  cfg,
		enabled: cfg.Enabled,
	}
}

func (p *Personalizer) IsEnabled() bool {
	return p.enabled
}

func (p *Personalizer) PersonalizeMessage(ctx context.Context, msg *models.Message) error {
	if !p.enabled {
		return nil
	}

	// Prepare the input for LLM
	input := types.PromptInput{
		UserPromptName: "email_personalization",
		Vars: map[string]interface{}{
			"subject":   msg.Subject,
			"body":      p.getBodyContent(msg),
			"recipient": p.getRecipientInfo(msg),
			"metadata":  msg.Metadata,
		},
	}

	// Create a custom action for email personalization
	action := blasterLLM.Action("personalize")

	// Get personalized content
	type PersonalizedEmail struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	result, err := blasterLLM.GetCompletion[PersonalizedEmail](ctx, action, input)
	if err != nil {
		log.Printf("Failed to personalize message %s: %v", msg.ID, err)
		return fmt.Errorf("LLM personalization failed: %w", err)
	}

	// Update the message with personalized content
	msg.Subject = result.Subject

	if msg.HTML != "" {
		msg.HTML = result.Body
	} else {
		msg.Text = result.Body
	}

	log.Printf("Successfully personalized message %s", msg.ID)
	return nil
}

func (p *Personalizer) getBodyContent(msg *models.Message) string {
	if msg.HTML != "" {
		return msg.HTML
	}
	return msg.Text
}

func (p *Personalizer) getRecipientInfo(msg *models.Message) map[string]interface{} {
	info := map[string]interface{}{
		"email": "",
		"name":  "",
	}

	if len(msg.To) > 0 {
		info["email"] = msg.To[0]
		// Extract name from email if possible
		if parts := strings.Split(msg.To[0], "@"); len(parts) > 0 {
			info["name"] = strings.ReplaceAll(parts[0], ".", " ")
		}
	}

	// Check if there's recipient info in metadata
	if recipientData, ok := msg.Metadata["recipient"]; ok {
		if recipientMap, ok := recipientData.(map[string]interface{}); ok {
			for k, v := range recipientMap {
				info[k] = v
			}
		}
	}

	return info
}

// PersonalizeWithCustomPrompt allows using a custom prompt for personalization
func (p *Personalizer) PersonalizeWithCustomPrompt(ctx context.Context, msg *models.Message, systemPrompt, userPrompt string) error {
	if !p.enabled {
		return nil
	}

	// Get the provider directly
	provider, err := blasterLLM.GetProvider(blasterLLM.Action("personalize"))
	if err != nil {
		return fmt.Errorf("failed to get LLM provider: %w", err)
	}

	// Get completion with custom prompts
	completion, err := provider.GetCompletion(ctx, "custom_personalization", systemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("failed to get completion: %w", err)
	}

	// Parse the completion content
	// Assuming the LLM returns the personalized email in a specific format
	content := completion.Content

	// Simple parsing - in production, you'd want more robust parsing
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		// First line as subject
		msg.Subject = strings.TrimPrefix(lines[0], "Subject: ")

		// Rest as body
		if len(lines) > 1 {
			body := strings.Join(lines[1:], "\n")
			body = strings.TrimSpace(body)

			if msg.HTML != "" {
				msg.HTML = body
			} else {
				msg.Text = body
			}
		}
	}

	return nil
}
