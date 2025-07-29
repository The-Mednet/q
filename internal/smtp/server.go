package smtp

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"smtp_relay/internal/config"
	"smtp_relay/internal/gmail"
	"smtp_relay/internal/queue"
	"smtp_relay/pkg/models"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
)

type Server struct {
	config    *config.SMTPConfig
	queue     queue.Queue
	router    *gmail.WorkspaceRouter
	server    *smtp.Server
}

func NewServer(cfg *config.SMTPConfig, q queue.Queue, router *gmail.WorkspaceRouter) *Server {
	s := &Server{
		config: cfg,
		queue:  q,
		router: router,
	}

	backend := &Backend{queue: q, router: router}

	server := smtp.NewServer(backend)
	server.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server.Domain = cfg.Host
	server.ReadTimeout = cfg.ReadTimeout
	server.WriteTimeout = cfg.WriteTimeout
	server.MaxMessageBytes = cfg.MaxSize
	server.MaxRecipients = 100
	server.AllowInsecureAuth = true

	s.server = server
	return s
}

func (s *Server) Start() error {
	log.Printf("Starting SMTP server on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

func (s *Server) Stop() error {
	return s.server.Close()
}

type Backend struct {
	queue  queue.Queue
	router *gmail.WorkspaceRouter
}

func (b *Backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &Session{queue: b.queue, router: b.router}, nil
}

type Session struct {
	queue   queue.Queue
	router  *gmail.WorkspaceRouter
	from    string
	to      []string
	message *models.Message
}

func (s *Session) AuthPlain(username, password string) error {
	return nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	
	// Determine workspace for this sender
	var workspaceID string
	if s.router != nil {
		if workspace, err := s.router.RouteMessage(from); err == nil {
			workspaceID = workspace.ID
		} else {
			log.Printf("Warning: Could not route message from %s: %v", from, err)
		}
	}
	
	s.message = &models.Message{
		ID:          uuid.New().String(),
		From:        from,
		WorkspaceID: workspaceID,
		Status:      models.StatusQueued,
		QueuedAt:    time.Now(),
		Headers:     make(map[string]string),
		Metadata:    make(map[string]interface{}),
	}
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	if s.message == nil {
		return errors.New("no mail transaction in progress")
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	s.message.To = s.to

	if err := s.parseMessage(data); err != nil {
		return err
	}

	if err := s.queue.Enqueue(s.message); err != nil {
		return fmt.Errorf("failed to queue message: %w", err)
	}

	log.Printf("Message %s queued successfully", s.message.ID)
	return nil
}

func (s *Session) Reset() {
	s.from = ""
	s.to = nil
	s.message = nil
}

func (s *Session) Logout() error {
	return nil
}

func (s *Session) parseMessage(data []byte) error {
	lines := strings.Split(string(data), "\n")
	headers := true
	var body strings.Builder

	for _, line := range lines {
		if headers {
			if line == "" || line == "\r" {
				headers = false
				continue
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				switch strings.ToLower(key) {
				case "subject":
					s.message.Subject = value
				case "content-type":
					if strings.Contains(strings.ToLower(value), "text/html") {
						s.message.Headers["Content-Type"] = value
					}
				case "cc":
					s.message.CC = parseAddresses(value)
				case "bcc":
					s.message.BCC = parseAddresses(value)
				case "x-campaign-id":
					s.message.CampaignID = value
				case "x-user-id":
					s.message.UserID = value
				default:
					s.message.Headers[key] = value

					// Extract recipient metadata from X-Recipient-* headers
					if strings.HasPrefix(strings.ToLower(key), "x-recipient-") {
						if s.message.Metadata["recipient"] == nil {
							s.message.Metadata["recipient"] = make(map[string]interface{})
						}
						recipientKey := strings.TrimPrefix(strings.ToLower(key), "x-recipient-")
						if recipientMap, ok := s.message.Metadata["recipient"].(map[string]interface{}); ok {
							recipientMap[recipientKey] = value
						}
					}
				}
			}
		} else {
			body.WriteString(line)
			body.WriteString("\n")
		}
	}

	bodyContent := strings.TrimSpace(body.String())
	if isHTML(s.message.Headers["Content-Type"]) {
		s.message.HTML = bodyContent
	} else {
		s.message.Text = bodyContent
	}

	return nil
}

func parseAddresses(addresses string) []string {
	parts := strings.Split(addresses, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		addr := strings.TrimSpace(part)
		if addr != "" {
			result = append(result, addr)
		}
	}
	return result
}

func isHTML(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/html")
}
