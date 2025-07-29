package llm

import (
	"os"
)

func init() {
	// Set up default prompts for email personalization
	// The blaster LLM package will use these environment variables
	
	// Set the provider based on configuration
	if provider := os.Getenv("LLM_PROVIDER"); provider != "" {
		switch provider {
		case "openai":
			os.Setenv("REWRITE_PROVIDER", "openai")
			os.Setenv("PERSONALIZE_PROVIDER", "openai")
		case "azure_openai":
			os.Setenv("REWRITE_PROVIDER", "azure_openai")
			os.Setenv("PERSONALIZE_PROVIDER", "azure_openai")
		case "groq":
			os.Setenv("REWRITE_PROVIDER", "groq")
			os.Setenv("PERSONALIZE_PROVIDER", "groq")
		}
	}

	// Set up default system prompt for email personalization
	if os.Getenv("EMAIL_PERSONALIZATION_SYSTEM_PROMPT") == "" {
		os.Setenv("EMAIL_PERSONALIZATION_SYSTEM_PROMPT", `You are an email personalization assistant. Your task is to personalize emails while maintaining their core message and intent. 

Guidelines:
1. Keep the same overall message and call-to-action
2. Adjust tone and language to be more personal and engaging
3. Use recipient information when available
4. Maintain professionalism
5. Keep the email concise and clear
6. Return the personalized email in JSON format with "subject" and "body" fields`)
	}

	// Set up default user prompt template
	if os.Getenv("EMAIL_PERSONALIZATION_USER_PROMPT") == "" {
		os.Setenv("EMAIL_PERSONALIZATION_USER_PROMPT", `Personalize the following email:

Subject: {{.subject}}

Body:
{{.body}}

Recipient Information:
{{.recipient}}

Additional Context:
{{.metadata}}

Please personalize this email while maintaining its core message. Return the result as JSON with "subject" and "body" fields.`)
	}
}