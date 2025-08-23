# Header Rewrite Test Scenarios

## Test Case 1: Replace Existing Header
**Input Message:**
```
From: sender@mail.example.com
To: recipient@example.com
Subject: Test Email
List-Unsubscribe: <https://mandrillapp.com/unsubscribe/abc123>

This is a test email.
```

**Workspace Config:**
```json
{
  "header_rewrite": {
    "enabled": true,
    "rules": [
      {
        "header_name": "List-Unsubscribe",
        "new_value": "<https://mail.example.com/unsubscribe?token=%recipient%>"
      }
    ]
  }
}
```

**Expected Result:**
- ✅ `List-Unsubscribe` header is **replaced** with `<https://mail.example.com/unsubscribe?token=%recipient%>`
- 📝 Log: "Replaced header List-Unsubscribe in workspace..."

## Test Case 2: Add Missing Header
**Input Message:**
```
From: sender@mail.example.com
To: recipient@example.com
Subject: Test Email

This is a test email with no List-Unsubscribe header.
```

**Workspace Config:**
```json
{
  "header_rewrite": {
    "enabled": true,
    "rules": [
      {
        "header_name": "List-Unsubscribe",
        "new_value": "<https://mail.example.com/unsubscribe?token=%recipient%>"
      },
      {
        "header_name": "X-Mailer",
        "new_value": "Mailgun/SMTP-Relay"
      }
    ]
  }
}
```

**Expected Result:**
- ✅ `List-Unsubscribe` header is **added** with value `<https://mail.example.com/unsubscribe?token=%recipient%>`
- ✅ `X-Mailer` header is **added** with value `Mailgun/SMTP-Relay`
- 📝 Log: "Added missing header List-Unsubscribe to workspace..."
- 📝 Log: "Added missing header X-Mailer to workspace..."

## Test Case 3: Mixed Scenario
**Input Message:**
```
From: sender@mail.example.com
To: recipient@example.com
Subject: Test Email
List-Unsubscribe: <https://old-provider.com/unsubscribe>

This email has some headers but not others.
```

**Workspace Config:**
```json
{
  "header_rewrite": {
    "enabled": true,
    "rules": [
      {
        "header_name": "List-Unsubscribe",
        "new_value": "<https://mail.example.com/unsubscribe?token=%recipient%>"
      },
      {
        "header_name": "List-Unsubscribe-Post",
        "new_value": "List-Unsubscribe=One-Click"
      }
    ]
  }
}
```

**Expected Result:**
- ✅ `List-Unsubscribe` header is **replaced** (existing header)
- ✅ `List-Unsubscribe-Post` header is **added** (missing header)
- 📝 Log: "Replaced header List-Unsubscribe in workspace..."
- 📝 Log: "Added missing header List-Unsubscribe-Post to workspace..."

## Test Case 4: Gmail Workspace (No Changes)
**Input Message:**
```
From: sender@example.com
To: recipient@example.com
Subject: Test Email
List-Unsubscribe: <https://mandrillapp.com/unsubscribe/abc123>

This email should pass through unchanged.
```

**Expected Result:**
- ✅ All headers remain **unchanged** (Gmail workspaces don't apply header rewriting)
- ✅ `List-Unsubscribe` remains `<https://mandrillapp.com/unsubscribe/abc123>`