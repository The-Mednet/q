package queue

import (
	"fmt"
	"sync"
	"time"

	"relay/pkg/models"
)

type MemoryQueue struct {
	mu       sync.RWMutex
	messages map[string]*models.Message
	order    []string
}

func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		messages: make(map[string]*models.Message),
		order:    make([]string, 0),
	}
}

func (q *MemoryQueue) Enqueue(message *models.Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.messages[message.ID] = message
	q.order = append(q.order, message.ID)
	return nil
}

func (q *MemoryQueue) Dequeue(batchSize int) ([]*models.Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var result []*models.Message
	var newOrder []string

	count := 0
	for _, id := range q.order {
		msg, exists := q.messages[id]
		if !exists {
			continue
		}

		if (msg.Status == models.StatusQueued || msg.Status == models.StatusFailed || msg.Status == models.StatusAuthError) && count < batchSize {
			msg.Status = models.StatusProcessing
			result = append(result, msg)
			count++
		}
		newOrder = append(newOrder, id)
	}

	q.order = newOrder
	return result, nil
}

func (q *MemoryQueue) UpdateStatus(id string, status models.MessageStatus, err error) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	msg, exists := q.messages[id]
	if !exists {
		return fmt.Errorf("message %s not found", id)
	}

	msg.Status = status
	now := time.Now()
	msg.ProcessedAt = &now

	if err != nil {
		msg.Error = err.Error()
	}

	return nil
}

func (q *MemoryQueue) Get(id string) (*models.Message, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	msg, exists := q.messages[id]
	if !exists {
		return nil, fmt.Errorf("message %s not found", id)
	}

	return msg, nil
}

func (q *MemoryQueue) Remove(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.messages, id)

	newOrder := make([]string, 0)
	for _, msgID := range q.order {
		if msgID != id {
			newOrder = append(newOrder, msgID)
		}
	}
	q.order = newOrder

	return nil
}

func (q *MemoryQueue) Close() error {
	return nil
}

func (q *MemoryQueue) GetStats() (map[string]interface{}, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := make(map[string]interface{})
	statusCounts := make(map[string]int)

	for _, msg := range q.messages {
		statusCounts[string(msg.Status)]++
	}

	var counts []map[string]interface{}
	for status, count := range statusCounts {
		counts = append(counts, map[string]interface{}{
			"Status": status,
			"Count":  count,
		})
	}

	stats["statusCounts"] = counts
	stats["total"] = len(q.messages)

	return stats, nil
}

func (q *MemoryQueue) GetMessages(limit, offset int, status string) ([]*models.Message, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var filtered []*models.Message

	for i := len(q.order) - 1; i >= 0; i-- {
		msg := q.messages[q.order[i]]
		if status == "" || status == "all" || string(msg.Status) == status {
			filtered = append(filtered, msg)
		}
	}

	start := offset
	if start > len(filtered) {
		return []*models.Message{}, nil
	}

	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end], nil
}

// GetSentCountsByWorkspaceAndSender returns sent message counts for the last 24 hours
func (q *MemoryQueue) GetSentCountsByWorkspaceAndSender() (map[string]map[string]int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	counts := make(map[string]map[string]int)
	cutoff := time.Now().Add(-24 * time.Hour)

	for _, msg := range q.messages {
		if msg.Status == models.StatusSent && msg.ProcessedAt != nil && msg.ProcessedAt.After(cutoff) {
			if counts[msg.WorkspaceID] == nil {
				counts[msg.WorkspaceID] = make(map[string]int)
			}
			counts[msg.WorkspaceID][msg.From]++
		}
	}

	return counts, nil
}
