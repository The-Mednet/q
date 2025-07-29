# Variable Replacement System

The SMTP relay service supports dynamic variable replacement in email content, allowing you to insert trending questions and other dynamic content into emails before they are sent.

## Configuration

To enable variable replacement, configure the Blaster API connection in your `.env` file:

```bash
BLASTER_BASE_URL=http://localhost:3034
BLASTER_API_KEY=your-blaster-api-key
```

## Supported Variables

### `<<TRENDING_QUESTION>>`

Inserts trending medical questions from the Blaster API into email content. The variable supports multiple parameter formats:

#### Basic Usage (No Parameters)
```
<<TRENDING_QUESTION>>
```
Uses user ID from message metadata if available, otherwise returns an error.

#### User-Based Trending
```
<<TRENDING_QUESTION:user:12345>>
```
Gets trending questions based on the specified user's specialty and subspecialty topics.

#### Topic-Based Trending
```
<<TRENDING_QUESTION:115,304>>
```
Gets trending questions for the specified topic IDs (comma-separated).

#### Question-Based Trending
```
<<TRENDING_QUESTION:question:12345,67890>>
```
Gets trending content for the specified question IDs (comma-separated). 

**Note:** This requires API updates on the blaster server side to handle the `/trending/questions/summary` endpoint.

## Priority and Fallback Strategy

The system tries different trending approaches in this order:

1. **Question-based trending** (if question IDs provided)
2. **User-based trending** (if user ID provided)  
3. **Topic-based trending** (if topic IDs provided)
4. **Error** if no valid parameters

If any method fails, it falls back to the next available method with appropriate logging.

## Output Format

The trending question variable is replaced with formatted content including:

- **Thread title** with fire emoji (🔥)
- **Summary** with expert quotes
- **Expert attribution** (name and level)
- **Topic context** 
- **Call-to-action link** to the discussion

Example output:
```
🔥 **What are your top takeaways in Thoracic Cancers from ASCO 2025?**

Join a timely discussion on the latest breakthroughs in thoracic cancers from ASCO 2025, including new immunotherapeutic agents and pivotal trial results. As Dr. Jarushka Naidoo notes, 'Tarlatamab in ES-SCLC [is the] first new agent approved for 2L SCLC in many years,' highlighting the rapid evolution in treatment options.

— Dr. Jarushka Naidoo

*From: Medical Oncology*

**[Join the discussion →](https://mednet.com/questions/24650)**
```

## Usage in Email Templates

Variables can be used in any part of the email:

### Subject Line
```
Subject: Weekly Digest: <<TRENDING_QUESTION:user:12345>>
```

### HTML Body
```html
<h2>This Week's Trending Discussion</h2>
<div class="trending-content">
  <<TRENDING_QUESTION:115,304>>
</div>
```

### Text Body
```
Here's what's trending in your specialty:

<<TRENDING_QUESTION>>

Best regards,
The Mednet Team
```

## Message Metadata

For user-based trending without explicit parameters, include user information in the message metadata:

```json
{
  "recipient": {
    "user_id": 12345,
    "email": "doctor@example.com",
    "name": "Dr. Smith"
  }
}
```

## Error Handling

The variable replacement system is designed to be robust:

- **API failures**: Continue with original content, log warning
- **Invalid parameters**: Log warning, skip replacement
- **Network issues**: Continue with original content, log error
- **Missing configuration**: Disable variable replacement entirely

All errors are logged but do not prevent email delivery.

## Processing Order

Variable replacement occurs **before** LLM personalization in the email processing pipeline:

1. **Variable Replacement** → Replace `<<VARIABLES>>` with dynamic content
2. **LLM Personalization** → Personalize the complete message content  
3. **Gmail Delivery** → Send the final processed email

This ensures that LLM personalization can work with the dynamically inserted content.

## Development and Testing

### Mock Server Testing

The system includes comprehensive tests with mock API responses. Run tests with:

```bash
go test ./tests -v -run TestTrendingVariableReplacement
go test ./tests -v -run TestVariableDetection
```

### Local Development

For local development, ensure the Blaster API is running and accessible:

```bash
# Check API health
curl -H "x-api-key: your-key" http://localhost:3034/health

# Test trending endpoint
curl -H "x-api-key: your-key" "http://localhost:3034/trending/summary?topicIds=115"
```

### Adding New Variables

To add new variables:

1. Update the `getVariableReplacement` method in `internal/variables/replacer.go`
2. Add processing logic for the new variable
3. Create tests in `tests/trending_variable_test.go`
4. Update this documentation

## API Endpoints Used

The variable replacement system calls these Blaster API endpoints:

- `GET /trending/summary?topicIds=...` - Topic-based trending
- `GET /trending/user/summary?userId=...` - User-based trending  
- `GET /trending/questions/summary?questionIds=...` - Question-based trending (**requires API implementation**)

All endpoints require the `x-api-key` header for authentication.

## Performance Considerations

- API calls have a 30-second timeout
- Failed API calls fall back gracefully
- Variable replacement is processed per email, not batched
- Consider rate limiting for high-volume email campaigns